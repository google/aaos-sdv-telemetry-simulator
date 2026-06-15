// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package finisher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/googleapi"
	"simulator.code/finish-simulation/compute"
	"simulator.code/finish-simulation/firestore"
)

// FinishRequest defines the expected JSON body for a finish request.
type FinishRequest struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Zone       string `json:"zone"`
	InstanceID string `json:"instance_id"`
	DocumentID string `json:"document_id"`
	Status     string `json:"status"`
}

// Finisher defines the interface for the core simulation finalization logic.
type Finisher interface {
	FinishSimulation(ctx context.Context, projectID string, zone string, instanceID string, documentID string, simulationStatus firestore.SimulationStatus) error
}

type finisher struct {
	firestoreClient firestore.Client
	computeClient   compute.Client
	logger          *slog.Logger
}

type firestoreClient interface {
	DecrementRunningVMsCounter(ctx context.Context) error
	GetSimulation(ctx context.Context, id string) (*firestore.SimulationDoc, error)
	UpdateStatus(ctx context.Context, id string, status firestore.SimulationStatus) error
	Close() error
}

type computeClient interface {
	GetInstance(ctx context.Context, projectID string, zone string, instanceID string) (*computepb.Instance, error)
	DeleteInstance(ctx context.Context, projectID string, zone string, instanceID string) error
	QueryInstances(ctx context.Context, projectID string, zone string, duration int) ([]string, error)
	Close() error
}

// NewFinisher creates a new Finisher.
func NewFinisher(fs firestoreClient, c computeClient, logger *slog.Logger) Finisher {
	return &finisher{
		firestoreClient: fs,
		computeClient:   c,
		logger:          logger,
	}
}

// FinishSimulation is the entry point for the Cloud Function.
func FinishSimulation(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for local development and web app calls
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	baseLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With(
		"function_name", "delete-simulation",
		"product", "cloud-telemetry-simulator",
	)

	fr := FinishRequest{}
	if err := json.NewDecoder(r.Body).Decode(&fr); err != nil {
		http.Error(w, "Error parsing request body", http.StatusBadRequest)
		baseLogger.Error("Error parsing request body", "error", err)
		return
	}

	var requestLogger *slog.Logger
	if fr.ID != "" {
		requestLogger = baseLogger.With("simulation_id", fr.ID)
	} else {
		requestLogger = baseLogger
		requestLogger.Info("Received request with empty body, treating as scheduled cleanup job.")
	}
	// Respond quickly to the client that the request has been accepted.
	// The actual work will be done in the background.
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Finalization request accepted and is being processed."))

	// Use a background context for the asynchronous processing.
	ctx := context.Background()

	projectID := os.Getenv("GCP_PROJECT")
	databaseID := os.Getenv("FIRESTORE_DATABASE")

	fsClient, err := firestore.NewClient(ctx, projectID, databaseID)
	if err != nil {
		requestLogger.Error("Failed to create Firestore client", "error", err)
		return
	}
	defer fsClient.Close()

	cClient, err := compute.NewClient(ctx, requestLogger)
	if err != nil {
		requestLogger.Error("Failed to create Compute client", "error", err)
		return
	}
	defer cClient.Close()

	finisher := NewFinisher(fsClient, cClient, requestLogger)

	if err := finisher.FinishSimulation(ctx, fr.ProjectID, fr.Zone, fr.InstanceID, fr.DocumentID, firestore.SimulationStatus(fr.Status)); err != nil {
		requestLogger.Error("Error during simulation finalization", "error", err, "request", fr)
		// The response has already been sent, so we just log the error.
	}
}

// FinishSimulation performs the actual work of updating status, decrementing counters, and deleting the VM.
func (f *finisher) FinishSimulation(ctx context.Context, projectID string, zone string, instanceID string, documentID string, simulationStatus firestore.SimulationStatus) error {
	f.logger.Info("Verifying instance identity", "instance_id", instanceID, "document_id", documentID)

	// Get instance details to verify its metadata against the request payload.
	// This ensures the agent making the call is the one assigned to this simulation.
	instance, err := f.computeClient.GetInstance(ctx, projectID, zone, instanceID)
	if err != nil {
		// If instance is not found, it might have been deleted already. Log and proceed with DB cleanup.
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
			f.logger.Warn("Instance not found during verification. Proceeding with database cleanup.", "instance_id", instanceID)
		} else {
			return fmt.Errorf("failed to get instance details for verification: %w", err)
		}
	}

	if instance != nil && instance.Metadata != nil {
		var metadataDocumentID string
		for _, item := range instance.Metadata.Items {
			if item.GetKey() == "document_id" {
				metadataDocumentID = item.GetValue()
				break
			}
		}

		if metadataDocumentID == "" || metadataDocumentID != documentID {
			f.logger.Error("Mismatch between request DocumentID and instance metadata DocumentID. Aborting.", "request_doc_id", documentID, "metadata_doc_id", metadataDocumentID)
			return fmt.Errorf("document ID mismatch for instance %s", instanceID)
		}
		f.logger.Info("Instance identity verified successfully", "instance_id", instanceID)
	}

	f.logger.Info("Processing finish request", "document_id", documentID)

	simulation, err := f.firestoreClient.GetSimulation(ctx, documentID)
	if err != nil {
		return fmt.Errorf("Simulation not found in Database %s: %w", instanceID, err)
	}

	if simulation.Status == firestore.StatusRunning {
		// 1. Update Firestore Status
		status := firestore.SimulationStatus(simulationStatus)
		if err := f.firestoreClient.UpdateStatus(ctx, documentID, status); err != nil {
			// Log the error but continue cleanup
			f.logger.Error("Failed to update simulation status", "document_id", documentID, "error", err)
			// We don't return here because cleanup should still proceed.
		} else {
			f.logger.Info("Successfully updated status", "status", status, "document_id", documentID)
		}

		// 2. Decrement Firestore Counter
		if err := f.firestoreClient.DecrementRunningVMsCounter(ctx); err != nil {
			// Log the error but continue cleanup
			f.logger.Warn("Failed to decrement running VM counter", "error", err)
			// We don't return here because cleanup should still proceed.
		} else {
			f.logger.Info("Successfully decremented running VM counter")
		}

		// 3. Delete Compute Instance
		if err := f.computeClient.DeleteInstance(ctx, projectID, zone, instanceID); err != nil {
			// This is a more critical failure.
			return fmt.Errorf("failed to delete instance %s: %w", instanceID, err)
		}
	}

	f.logger.Info("Successfully initiated deletion for instance", "instance_id", instanceID)
	return nil
}
