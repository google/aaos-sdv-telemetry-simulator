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

package deletion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	compute "simulator.code/delete-simulation/compute"
	firestore "simulator.code/delete-simulation/firestore"
)

// Environment variable names
const (
	EnvGCPProject        = "GCP_PROJECT"
	EnvComputeZone       = "COMPUTE_ZONE"
	EnvFirestoreDatabase = "FIRESTORE_DATABASE"
)


type CancellationRequest struct {
	ID string `json:"id"`
}

type canceller struct {
	firestoreClient firestoreClient
	computeClient   computeClient
	logger          *slog.Logger
}

type Canceller interface {
	CancelRequest(ctx context.Context, id string, projectID string, zone string) error
	Cleanup(ctx context.Context, projectID string, zone string) error
	CleanupAfterCancellation(ctx context.Context, simulation_id string, projectID string, zone string) error
}

type firestoreClient interface {
	GetSimulation(ctx context.Context, id string) (*firestore.SimulationDoc, error)
	UpdateCounter(ctx context.Context) error
	GetStaleSimulations(ctx context.Context, ageInHours int) ([]*firestore.SimulationDoc, error)
	UpdateStatus(ctx context.Context, id string, status firestore.SimulationStatus) error
	Close() error
}

type computeClient interface {
	DeleteInstance(ctx context.Context, projectID string, zone string, instanceID string) error
	InstanceExists(ctx context.Context, projectID string, zone string, instanceID string) (bool, error)
	QueryInstances(ctx context.Context, projectID string, zone string, duration int) ([]string, error)
	Close() error
}

func NewCanceller(fs firestoreClient, c computeClient, logger *slog.Logger) Canceller {
	return &canceller{
		firestoreClient: fs,
		computeClient:   c,
		logger:          logger,
	}
}

func (c *canceller) Cleanup(ctx context.Context, projectID string, zone string) error {
	c.logger.Info("Starting cleanup", "project", projectID, "zone", zone)
	if c == nil {
		return fmt.Errorf("Canceller is nil")
	}
	if c.firestoreClient == nil {
		return fmt.Errorf("Firestore client is not initialized")
	}
	if c.computeClient == nil {
		return fmt.Errorf("Compute client is not initialized")
	}

	const cleanupAgeHours = 24

	// Step 1: Find stale documents in Firestore and update their status if the VM is gone.
	staleDocs, err := c.firestoreClient.GetStaleSimulations(ctx, cleanupAgeHours)
	if err != nil {
		return fmt.Errorf("failed querying for stale simulation documents: %w", err)
	}
	c.logger.Info("Found potentially stale simulation documents", "count", len(staleDocs))

	for _, doc := range staleDocs {
		exists, err := c.computeClient.InstanceExists(ctx, projectID, zone, doc.InstanceID)
		if err != nil {
			c.logger.Warn("Failed to check for instance existence during cleanup, skipping", "instance", doc.InstanceID, "error", err)
			continue
		}
		if !exists {
			c.logger.Info("Found stale simulation document for a non-existent VM. Updating status.", "instance", doc.InstanceID, "document_id", doc.DocumentID)
			if err := c.firestoreClient.UpdateStatus(ctx, doc.DocumentID, firestore.StatusCancelled); err != nil {
				// Log error but continue, to not block other cleanups.
				c.logger.Error("Failed updating status for stale simulation document", "document_id", doc.DocumentID, "error", err)
			}
		}
	}

	// Step 2: Find long-running VM instances and delete them.
	results, err := c.computeClient.QueryInstances(ctx, projectID, zone, cleanupAgeHours)
	if err != nil {
		return fmt.Errorf("Failed querying for running simulation instances: %w", err)
	}
	c.logger.Info("Found instances to clean up", "count", len(results))

	for _, instanceID := range results {
		doc, err := c.firestoreClient.GetSimulation(ctx, instanceID)
		if err != nil {
			c.logger.Warn("Failed getting simulation document for instance from firestore during cleanup", "instance", instanceID, "error", err)
			continue // Continue to the next instance even if this one fails
		}
		if doc == nil {
			c.logger.Info("Simulation document for instance not found during cleanup. Skipping.", "instance", instanceID)
			continue
		}
		c.logger.Info("Deleting instance", "instance", instanceID)
		err = c.computeClient.DeleteInstance(ctx, projectID, zone, instanceID)
		if err != nil {
			return fmt.Errorf("Failed deleting instance: %w", err)
		}
		err = c.firestoreClient.UpdateStatus(ctx, doc.DocumentID, firestore.StatusCancelled)
		if err != nil {
			return fmt.Errorf("Failed updating simulation status: %w", err)
		}
	}
	c.logger.Info("Cleanup completed successfully")
	return nil
}

func (c *canceller) CancelRequest(ctx context.Context, simulation_id string, projectID string, zone string) error {
	c.logger.Info("Starting cancellation request", "project", projectID, "zone", zone)
	if c == nil {
		return fmt.Errorf("Canceller is nil")
	}
	if c.firestoreClient == nil {
		return fmt.Errorf("Firestore client is not initialized")
	}
	if c.computeClient == nil {
		return fmt.Errorf("Compute client is not initialized")
	}

	doc, err := c.firestoreClient.GetSimulation(ctx, simulation_id)
	if err != nil {
		return fmt.Errorf("Error getting simulation: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("Simulation document is nil")
	}

	switch doc.Status {
	case firestore.StatusRunning, firestore.StatusScheduled:
		c.logger.Info("Updating status to cancelled")
		if err := c.firestoreClient.UpdateStatus(ctx, doc.DocumentID, firestore.StatusCancelled); err != nil {
			return fmt.Errorf("Failed to update status: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("Simulation cannot be cancelled: invalid status %v", doc.Status)
	}
}

func (c *canceller) CleanupAfterCancellation(ctx context.Context, simulation_id string, projectID string, zone string) error {
	doc, err := c.firestoreClient.GetSimulation(ctx, simulation_id)
	if err != nil {
		return fmt.Errorf("Error getting simulation during cleanup: %w", err)
	}
	if doc == nil {
		c.logger.Info("Simulation document not found during cleanup. Skipping instance deletion.")
		return nil // Nothing to clean up if the document doesn't exist
	}

	if err := c.computeClient.DeleteInstance(ctx, projectID, zone, doc.InstanceID); err != nil {
		return fmt.Errorf("Failed to delete compute instance %s: %w", doc.InstanceID, err)
	}
	c.logger.Info("Successfully deleted compute instance", "instance", doc.InstanceID)

	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		if err := c.firestoreClient.UpdateCounter(ctx); err != nil {
			if i+1 == maxRetries {
				return fmt.Errorf("Max retries reached for updating counter")
			}
			time.Sleep(time.Second * 2) // Wait before retrying
			continue
		}

		c.logger.Info("Successfully updated counter in Firestore")
		break
	}

	c.logger.Info("Cleanup completed")
	return nil
}

// handleCleanupJob processes requests from a scheduled cleanup job.
func handleCleanupJob(ctx context.Context, canceller Canceller, projectID, zone string, logger *slog.Logger) {
	logger.Info("Starting scheduled cleanup job", "project", projectID, "zone", zone)
	err := canceller.Cleanup(ctx, projectID, zone)
	if err != nil {
		logger.Error("Error during scheduled cleanup", "error", err)
	} else {
		logger.Info("Scheduled cleanup job completed successfully.")
	}
}

// handleSpecificCancellation processes a specific simulation cancellation request.
func handleSpecificCancellation(ctx context.Context, canceller Canceller, simulationID, projectID, zone string, logger *slog.Logger) {
	logger.Info("Processing cancellation request", "project", projectID, "zone", zone)

	err := canceller.CancelRequest(ctx, simulationID, projectID, zone)
	if err != nil {
		logger.Error("Error cancelling request for simulation", "error", err)
		return
	}
	logger.Info("Simulation status updated to cancelled. Proceeding with cleanup.")
	if err := canceller.CleanupAfterCancellation(ctx, simulationID, projectID, zone); err != nil {
		logger.Error("Error cleaning up after cancellation request for simulation", "error", err)
	} else {
		logger.Info("Cleanup after cancellation completed.")
	}
}

// initClients initializes Firestore and Compute clients.
func initClients(ctx context.Context, projectID, databaseID string, logger *slog.Logger) (firestoreClient, computeClient, error) {
	fsClient, err := firestore.NewClient(ctx, projectID, databaseID, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating firestore client: %w", err)
	}
	cClient, err := compute.NewClient(ctx, logger)
	if err != nil {
		fsClient.Close() // Close firestore client if compute client creation fails
		return nil, nil, fmt.Errorf("error creating compute client: %w", err)
	}
	return fsClient, cClient, nil
}

func HandleCancellationRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create a base logger with common attributes
	baseLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With(
		"function_name", "delete-simulation",
		"product", "cloud-telemetry-simulator",
	)

	cr := CancellationRequest{}
	// Decode request only if body is non-empty. The body contains the ID to be deleted in the case
	// of a specific cancellation request. Otherwise, it's a cleanup request.
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&cr); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	// Create a request-specific logger, potentially adding simulation_id
	var requestLogger *slog.Logger
	if cr.ID != "" {
		requestLogger = baseLogger.With("simulation_id", cr.ID)
	} else {
		requestLogger = baseLogger
		requestLogger.Info("Received request with empty body, treating as scheduled cleanup job.")
	}

	// Respond quickly to the client
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Request accepted and is being processed"))

	ctx := context.Background()

	projectID := os.Getenv(EnvGCPProject)
	zone := os.Getenv(EnvComputeZone)
	databaseID := os.Getenv(EnvFirestoreDatabase)

	fsClient, cClient, err := initClients(ctx, projectID, databaseID, requestLogger)
	if err != nil {
		requestLogger.Error("Failed to initialize clients", "error", err)
		return
	}
	defer fsClient.Close()
	defer cClient.Close()

	canceller := NewCanceller(fsClient, cClient, requestLogger)

	if cr.ID == "" {
		handleCleanupJob(ctx, canceller, projectID, zone, requestLogger)
	} else {
		handleSpecificCancellation(ctx, canceller, cr.ID, projectID, zone, requestLogger)
	}
}
