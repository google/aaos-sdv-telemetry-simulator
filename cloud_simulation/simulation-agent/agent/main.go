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

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"simulator.code/simulation-agent/adb"
	"simulator.code/simulation-agent/finisher"
	"simulator.code/simulation-agent/storage"
)

var metadataBaseURL = "http://metadata.google.internal/computeMetadata/v1/instance/"

const (
	statusCompleted = "completed"
	statusFailed    = "failed"
)

type agent struct {
	storageClient     storage.Client
	finisherClient    finisher.Client
	adbClient         adb.Client
	metadataBaseURL   string
	simulationID      string
	simulationBucket  string
	instanceName      string
	configsPath       string
	outputsDir        string
	documentID        string
	maxSimulationTime int
	maxReportCount    int
	finishURL         string
	logger            *slog.Logger
}

type Agent interface {
	executeSimulation(ctx context.Context, projectID string, zone string) error
	startVM(withInstanceName bool) error
	waitForVM() error
	validateDir(dir string) error
	pushMetricsConfigs(metricsDir string) (string, error)
	pushPublisherConfigs(publishersDir string) (string, error)
	runTelemetrySimulator(metricsConfigs, publisherConfigs string, max_simulation_time, max_report_count int) error
	collectOutputFiles() error
}

type storageClient interface {
	Download(ctx context.Context, filePath string) error
	Upload(ctx context.Context, bucketPath, localDir string) error
	Close() error
}

type finisherClient interface {
	Finish(ctx context.Context, url string, payload finisher.Payload) error
}

type adbClient interface {
	StartServer() error
	LaunchCvd(withInstanceName bool) (io.ReadCloser, io.ReadCloser, error)
	StopCvd() error
	Connect() error
	Shell(args ...string) error
	Root() error
	Push(localSrc, deviceDst string) error
	Pull(deviceSrc, localDst string) error
	Logcat() (io.ReadCloser, io.ReadCloser, error)
	Bugreport(filePath string) error
}

func NewAgent(s storageClient, f finisherClient, a adbClient, md, sim, sb, instanceName, cp, od, docID, finishURL string, maxSimTime, maxRepCount int, logger *slog.Logger) Agent {
	return &agent{
		storageClient:     s,
		finisherClient:    f,
		adbClient:         a,
		metadataBaseURL:   md,
		simulationID:      sim,
		simulationBucket:  sb,
		instanceName:      instanceName,
		configsPath:       cp,
		outputsDir:        od,
		documentID:        docID,
		finishURL:         finishURL,
		maxSimulationTime: maxSimTime,
		maxReportCount:    maxRepCount,
		logger:            logger,
	}
}

func (a *agent) runSimulationLifecycle(ctx context.Context) error {
	a.logger.Info("Starting simulation lifecycle")
	// Download required input files
	if err := a.storageClient.Download(ctx, "gs://"+a.simulationBucket+"/simulations/"+a.simulationID+"/inputs/"); err != nil {
		return fmt.Errorf("error downloading simulation input data from object storage: %w", err)
	}
	a.logger.Info("Input files downloaded", "path", a.configsPath)

	// Run simulation business logic
	simulationErr := a.runSimulation(a.configsPath, a.maxSimulationTime, a.maxReportCount)

	// Always try to upload simulation results, even if the simulation failed.
	// This ensures logs and partial results are captured.
	if uploadErr := a.storageClient.Upload(ctx, "gs://"+a.simulationBucket+"/simulations/"+a.simulationID+"/outputs/", a.outputsDir); uploadErr != nil {
		a.logger.Error("Error uploading simulation results to object storage", "error", uploadErr)
		// If simulation also failed, wrap both errors.
		if simulationErr != nil {
			return fmt.Errorf("error running simulation: %w; and failed to upload results: %w", simulationErr, uploadErr)
		}
		return fmt.Errorf("error uploading simulation results to object storage: %w", uploadErr)
	}
	a.logger.Info("Simulation results uploaded", "from", a.outputsDir)

	if simulationErr != nil {
		return fmt.Errorf("error running simulation: %w", simulationErr)
	}

	return nil
}

func (a *agent) executeSimulation(ctx context.Context, projectID string, zone string) error {
	var simulationErr error
	// Defer the finalization step to ensure it runs regardless of success or failure.
	defer func() {
		status := statusCompleted
		if simulationErr != nil {
			status = statusFailed
		}
		a.logger.Info("Calling finish-simulation function to finalize and clean up resources", "status", status)
		// This is a best-effort call. If it fails, the agent has still done its job.
		// The instance will eventually be terminated by its maxRunDuration.
		finishPayload := finisher.Payload{
			ID:         a.simulationID,
			ProjectID:  projectID,
			Zone:       zone,
			InstanceID: a.instanceName,
			DocumentID: a.documentID,
			Status:     status,
		}
		if err := a.finisherClient.Finish(ctx, a.finishURL, finishPayload); err != nil {
			a.logger.Error("Failed to call finish-simulation function", "error", err)
		}
	}()

	a.logger.Info("Starting simulation execution", "document_id", a.documentID)

	// Run the core simulation logic.
	simulationErr = a.runSimulationLifecycle(ctx)
	return simulationErr
}

func getMetadata(client *http.Client, path string) (string, error) {
	req, err := http.NewRequest("GET", metadataBaseURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Add("Metadata-Flavor", "Google")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	return string(body), nil
}

func main() {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelInfo)
	if levelStr := os.Getenv("LOG_LEVEL"); levelStr != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(levelStr)); err == nil {
			logLevel.Set(level)
		}
	}
	baseLogger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})).With(
		"function_name", "agent",
		"product", "cloud-telemetry-simulator",
	)

	metadataClient := &http.Client{Timeout: 5 * time.Second}

	simulationID, err := getMetadata(metadataClient, "attributes/simulation_id")
	if err != nil {
		baseLogger.Error("Failed to get simulation ID metadata", "error", err)
		os.Exit(1)
	}
	if simulationID == "" {
		baseLogger.Error("Simulation ID metadata is empty")
		os.Exit(1)
	}

	logger := baseLogger.With("simulation_id", simulationID)

	projectID, err := getMetadata(metadataClient, "attributes/project_id")
	if err != nil {
		logger.Error("Failed to get project ID metadata", "error", err)
		os.Exit(1)
	}
	if projectID == "" {
		logger.Error("Project ID metadata is empty")
		os.Exit(1)
	}

	zone, err := getMetadata(metadataClient, "attributes/zone")
	if err != nil {
		logger.Error("Failed to get zone metadata", "error", err)
		os.Exit(1)
	}
	if zone == "" {
		logger.Error("Zone metadata is empty")
		os.Exit(1)
	}

	databaseID, err := getMetadata(metadataClient, "attributes/database_id")
	if err != nil {
		logger.Error("Failed to get database ID metadata", "error", err)
		os.Exit(1)
	}
	if databaseID == "" {
		logger.Error("Database ID metadata is empty")
		os.Exit(1)
	}

	documentID, err := getMetadata(metadataClient, "attributes/document_id")
	if err != nil || documentID == "" {
		logger.Error("Missing document ID metadata", "error", err)
		os.Exit(1)
	}
	logger.Info("Document ID", "id", documentID)

	maxSimTimeStr, err := getMetadata(metadataClient, "attributes/max_simulation_time")
	if err != nil || maxSimTimeStr == "" {
		logger.Error("Missing max_simulation_time metadata", "error", err)
		os.Exit(1)
	}
	maxSimTime, err := strconv.Atoi(maxSimTimeStr)
	if err != nil {
		logger.Error("Error converting max_simulation_time to int", "error", err)
		os.Exit(1)
	}

	maxRepCountStr, err := getMetadata(metadataClient, "attributes/max_report_count")
	if err != nil || maxRepCountStr == "" {
		logger.Error("Missing max_report_count metadata", "error", err)
		os.Exit(1)
	}
	maxRepCount, err := strconv.Atoi(maxRepCountStr)
	if err != nil {
		logger.Error("Error converting max_report_count to int", "error", err)
		os.Exit(1)
	}

	instanceName, err := getMetadata(metadataClient, "name")
	if err != nil || instanceName == "" {
		logger.Error("Missing instance name metadata", "error", err)
		os.Exit(1)
	}
	logger.Info("Instance Name", "name", instanceName)

	simulationBucket, err := getMetadata(metadataClient, "attributes/simulation_bucket")
	if err != nil {
		logger.Error("Failed to get simulations bucket metadata", "error", err)
		os.Exit(1)
	}
	if simulationBucket == "" {
		logger.Error("Simulations bucket metadata is empty")
		os.Exit(1)
	}
	logger.Info("Simulations bucket", "bucket", simulationBucket)

	finishURL, err := getMetadata(metadataClient, "attributes/finish_function_url")
	if err != nil || finishURL == "" {
		logger.Error("Missing finish_function_url metadata", "error", err)
		os.Exit(1)
	}
	logger.Info("Finish function URL", "url", finishURL)

	ctx := context.Background()
	sClient, err := storage.NewClient(ctx, logger)
	if err != nil {
		logger.Error("Error creating Storage client", "error", err)
		os.Exit(1)
	}
	defer sClient.Close()

	fClient, err := finisher.NewClient(ctx)
	if err != nil {
		logger.Error("Error creating Finisher client", "error", err)
		os.Exit(1)
	}

	aClient, err := adb.NewClient()
	if err != nil {
		logger.Error("Error creating ADB client", "error", err)
		os.Exit(1)
	}

	agent := NewAgent(
		sClient,
		fClient,
		aClient,
		metadataBaseURL,
		simulationID,
		simulationBucket,
		instanceName,
		"downloads/",
		"/outputs/",
		documentID,
		finishURL,
		maxSimTime,
		maxRepCount,
		logger,
	)

	logger.Info("Agent created", "agent", fmt.Sprintf("%+v", agent))

	err = agent.executeSimulation(ctx, projectID, zone)
	if err != nil {
		logger.Error("Simulation Agent execution finished with an error", "error", err)
		os.Exit(1)
	}
}
