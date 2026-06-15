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

package receiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"

	"github.com/google/uuid"
	"simulator.code/receive-request/firestore"

	"simulator.code/receive-request/pubsub"
	"simulator.code/receive-request/storage"
)

// SimulationRequest defines the expected JSON body for a new simulation request.
type SimulationRequest struct {
	Owner             string `json:"owner"`
	FilePath          string `json:"file_path"`
	BuildID           string `json:"build_id"`
	InstanceType      string `json:"instance_type"`
	MaxSimulationTime *int   `json:"max_simulation_time"` // Use pointer to distinguish between 0 and not set
	MaxReportCount    *int   `json:"max_report_count"`    // Use pointer to distinguish between 0 and not set
}

// Validate checks for the presence of all required fields.
func (sr *SimulationRequest) Validate() error {
	if sr.Owner == "" {
		return errors.New("Missing required field: owner")
	}
	if sr.FilePath == "" {
		return errors.New("Missing required field: file_path")
	}
	if sr.BuildID == "" {
		return errors.New("Missing required field: build_id")
	}
	if sr.InstanceType == "" {
		return errors.New("Missing required field: instance_type")
	}
	if sr.MaxSimulationTime == nil {
		return errors.New("Missing required field: max_simulation_time")
	}
	if sr.MaxReportCount == nil {
		return errors.New("Missing required field: max_report_count")
	}
	return nil
}

type receiver struct {
	requestID       string
	firestoreClient firestore.Client
	storageClient   storage.Client
	pubsubClient    pubsub.Client
	logger          *slog.Logger
}

type Receiver interface {
	ReceiveRequest(requestID string, w http.ResponseWriter, r *http.Request, topicID string, agentDockerImage string, simulationBucketPath string, supportedBuilds string) error
}

type firestoreClient interface {
	Write(ctx context.Context, record pubsub.AvroRecord) error
	Close() error
}

type storageClient interface {
	Validate(ctx context.Context, filePath string) error
	CopyToOutputs(ctx context.Context, inputfilePath string, destinationPath string) error
	Close() error
}

type pubsubClient interface {
	PublishMessage(ctx context.Context, topic string, message pubsub.AvroRecord) error
	Close() error
}

func NewReceiver(fs firestoreClient, s storageClient, ps pubsub.Client, logger *slog.Logger) Receiver {
	return &receiver{
		firestoreClient: fs,
		storageClient:   s,
		pubsubClient:    ps,
		logger:          logger,
	}
}

func (r *receiver) ReceiveRequest(requestID string, w http.ResponseWriter, req *http.Request, topicID string, agentDockerImage string, simulationBucketPath string, supportedBuilds string) error {
	ctx := req.Context()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return fmt.Errorf("Error reading request body: %w", err)
	}

	r.logger.Info("Received request body", "body", string(body))

	var reqData SimulationRequest
	err = json.Unmarshal(body, &reqData)
	if err != nil {
		http.Error(w, "Error parsing request body", http.StatusBadRequest)
		return fmt.Errorf("Error parsing request body: %w", err)
	}

	if err := reqData.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return err
	}

	fullBuildID := agentDockerImage + ":" + reqData.BuildID

	// Validate build_id against supported builds if provided
	if supportedBuilds != "" && supportedBuilds != "{}" {
		var supported map[string]string
		if err := json.Unmarshal([]byte(supportedBuilds), &supported); err != nil {
			r.logger.Error("Error parsing SUPPORTED_AGENT_BUILDS", "error", err)
			http.Error(w, "Internal server error: invalid build configuration", http.StatusInternalServerError)
			return fmt.Errorf("error parsing supported builds: %w", err)
		}

		if len(supported) > 0 {
			if fingerprint, ok := supported[reqData.BuildID]; ok {
				fullBuildID = agentDockerImage + "@" + fingerprint
			} else {
				keys := make([]string, 0, len(supported))
				for k := range supported {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				errMsg := fmt.Sprintf("Unsupported build_id: %q. Supported builds are: %v", reqData.BuildID, keys)
				http.Error(w, errMsg, http.StatusBadRequest)
				return errors.New(errMsg)
			}
		}
	}

	record := pubsub.AvroRecord{
		ID:                requestID,
		Owner:             reqData.Owner,
		FilePath:          simulationBucketPath + "/" + reqData.FilePath,
		BuildID:           fullBuildID,
		InstanceType:      reqData.InstanceType,
		MaxSimulationTime: *reqData.MaxSimulationTime,
		MaxReportCount:    *reqData.MaxReportCount,
	}

	if err := r.storageClient.Validate(ctx, record.FilePath); err != nil {
		http.Error(w, fmt.Sprintf("Error validating file path: %v", err), http.StatusBadRequest)
		return fmt.Errorf("Error validating file path: %w", err)
	}

	if err := r.firestoreClient.Write(ctx, record); err != nil {
		http.Error(w, fmt.Sprintf("Error writing to Firestore: %v", err), http.StatusInternalServerError)
		return fmt.Errorf("Error writing to Firestore: %w", err)
	}

	if err := r.storageClient.CopyToOutputs(ctx, record.FilePath, simulationBucketPath+"/simulations/"+record.ID+"/inputs/"); err != nil {
		http.Error(w, fmt.Sprintf("Error copying input files to simulation directory in cloud storage: %v", err), http.StatusInternalServerError)
		return fmt.Errorf("Error copying input files to simulation directory in cloud storage: %w", err)
	}

	if err := r.pubsubClient.PublishMessage(ctx, topicID, record); err != nil {
		http.Error(w, fmt.Sprintf("Error publishing Pub/Sub message: %v", err), http.StatusInternalServerError)
		return fmt.Errorf("Error publishing Pub/Sub message: %w", err)
	}

	// Respond with the generated ID
	response := map[string]string{"id": requestID}
	respJSON, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Error creating response", http.StatusInternalServerError)
		return fmt.Errorf("error creating response: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respJSON)

	r.logger.Info("Record successfully written to Firestore and published to Pub/Sub", "simulation_id", requestID)
	return nil
}

func HandleSimulationRequest(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	baseLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With(
		"function_name", "receive-request",
		"product", "cloud-telemetry-simulator",
	)

	ctx := r.Context()
	projectID := os.Getenv("GCP_PROJECT")
	databaseID := os.Getenv("FIRESTORE_DATABASE")
	topicID := os.Getenv("PUBSUB_TOPIC_ID")
	agentDockerImage := os.Getenv("AGENT_DOCKER_IMAGE")
	simulationBucketPath := os.Getenv("SIMULATION_BUCKET")
	supportedBuilds := os.Getenv("SUPPORTED_AGENT_BUILDS")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestID := uuid.New().String()
	var requestLogger *slog.Logger
	requestLogger = baseLogger.With("simulation_id", requestID)
	requestLogger.Info("Generated ID")

	fsClient, err := firestore.NewClient(ctx, projectID, databaseID)
	if err != nil {
		requestLogger.Error("Error creating Firestore client", "error", err)
		http.Error(w, fmt.Sprintf("Error creating Firestore client: %v", err), http.StatusInternalServerError)
		return
	}
	defer fsClient.Close()

	sClient, err := storage.NewClient(ctx, requestLogger)
	if err != nil {
		requestLogger.Error("Error creating Storage client", "error", err)
		http.Error(w, fmt.Sprintf("Error creating Storage client: %v", err), http.StatusInternalServerError)
		return
	}
	defer sClient.Close()

	psClient, err := pubsub.NewClient(ctx, projectID, requestLogger)
	if err != nil {
		requestLogger.Error("Error creating Pub/Sub client", "error", err)
		http.Error(w, fmt.Sprintf("Error creating Pub/Sub client: %v", err), http.StatusInternalServerError)
		return
	}
	defer psClient.Close()

	receiver := NewReceiver(fsClient, sClient, psClient, requestLogger)
	err = receiver.ReceiveRequest(requestID, w, r, topicID, agentDockerImage, simulationBucketPath, supportedBuilds)
	if err != nil {
		requestLogger.Error("Error processing request", "error", err)
	}
}
