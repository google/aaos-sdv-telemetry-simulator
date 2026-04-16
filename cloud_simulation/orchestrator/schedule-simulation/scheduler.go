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

package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"simulator.code/simulation-scheduler/compute"
	"simulator.code/simulation-scheduler/firestore"
	"simulator.code/simulation-scheduler/pubsub"
)


type scheduler struct {
	firestoreClient firestore.Client
	computeClient   compute.Client
	pubsubClient    pubsub.Client
	logger          *slog.Logger
}

type Scheduler interface {
	ScheduleSimulation(ctx context.Context, projectID string, zone string, databaseID string, maxRunningVMs int, subscription_id string, instanceConfig compute.InstanceConfig) error
}

type firestoreClient interface {
	CheckCounter(ctx context.Context, maxRunningVMs int) (bool, error)
	IncrementCounter(ctx context.Context, maxRunningVMs int) error
	DecrementCounter(ctx context.Context) error
	SimulationUpdate(ctx context.Context, id string, instance string, ip string) error
	GetSimulationDocID(ctx context.Context, simulationID string) (string, error)
	Close() error
}

type computeClient interface {
	CreateInstance(ctx context.Context, projectID string, zone string, databaseID string, config compute.InstanceConfig) (string, string, error)
	Close() error
}

type pubsubClient interface {
	PullMessage(ctx context.Context, subscription_id string) (*pubsub.AvroRecord, error)
	Close() error
}

func NewScheduler(fs firestoreClient, c computeClient, ps pubsubClient, logger *slog.Logger) Scheduler {
	return &scheduler{
		firestoreClient: fs,
		computeClient:   c,
		pubsubClient:    ps,
		logger:          logger,
	}
}

func (s *scheduler) ScheduleSimulation(ctx context.Context, projectID string, zone string, databaseID string, maxRunningVMs int, subscription_id string, instanceConfig compute.InstanceConfig) error {
	var canSchedule bool
	var err error
	for attempts := 0; attempts < 3; attempts++ {
		canSchedule, err = s.firestoreClient.CheckCounter(ctx, maxRunningVMs)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(attempts) * 100 * time.Millisecond)
	}

	if err != nil {
		return fmt.Errorf("Error checking counter after retries: %w", err)
	}

	if !canSchedule {
		s.logger.Info("Maximum running VMs reached. Not scheduling new simulation.", "max_vms", maxRunningVMs)
		return nil
	}

	message, err := s.pubsubClient.PullMessage(ctx, subscription_id)
	if err != nil {
		return err
	}

	if message == nil {
		s.logger.Info("No more messages in pub/sub.")
		return nil
	}
	docID, err := s.firestoreClient.GetSimulationDocID(ctx, message.ID)
	if err != nil {
		return fmt.Errorf("failed to get document ID for simulation %s: %w", message.ID, err)
	}
	s.logger.Info("Pulled pub/sub message", "message", message, "simulation_id", message.ID)

	err = s.firestoreClient.IncrementCounter(ctx, maxRunningVMs)
	if err != nil {
		return err
	}

	instanceConfig.DocumentID = docID
	instanceConfig.SimulationID = message.ID
	instanceConfig.DockerImage = message.BuildID
	instanceConfig.InstanceType = message.InstanceType
	instanceConfig.MaxSimulationTime = message.MaxSimulationTime
	instanceConfig.MaxReportCount = message.MaxReportCount

	instanceID, ip, err := s.computeClient.CreateInstance(ctx, projectID, zone, databaseID, instanceConfig)
	if err != nil {
		dec_err := s.firestoreClient.DecrementCounter(ctx)
		if dec_err != nil {
			return fmt.Errorf("Error creating instance: %v, decrementing counter failed: %w", err, dec_err)
		}
		return fmt.Errorf("Error creating instance: %w", err)
	}
	s.logger.Info("Instance created", "ip", ip, "simulation_id", instanceConfig.SimulationID)

	err = s.firestoreClient.SimulationUpdate(ctx, instanceConfig.SimulationID, instanceID, ip)
	if err != nil {
		return fmt.Errorf("Error updating Simulation's Firestore document: %w", err)
	}

	s.logger.Info("ScheduleSimulation for request finished successfully", "simulation_id", instanceConfig.SimulationID)
	return nil
}

func HandleScheduleTrigger(w http.ResponseWriter, r *http.Request) {
	// Send HTTP response immediately
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	baseLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With(
		"function_name", "schedule-simulation",
		"product", "cloud-telemetry-simulator",
	)

	projectID := os.Getenv("GCP_PROJECT")
	zone := os.Getenv("COMPUTE_ZONE")
	databaseID := os.Getenv("FIRESTORE_DATABASE")
	subscriptionId := os.Getenv("PUBSUB_SUBSCRIPTION")
	maxRunningVMs, err := strconv.Atoi(os.Getenv("MAX_VM_COUNT"))
	if err != nil {
		baseLogger.Error("Error parsing MAX_VM_COUNT", "error", err)
		return
	}

	var instanceConfig compute.InstanceConfig
	instanceConfig.RegistryRegion = os.Getenv("REGISTRY_REGION")
	instanceConfig.ServiceAccount = os.Getenv("VM_SERVICEACCOUNT")
	instanceConfig.VMImage = os.Getenv("VM_IMAGE")
	instanceConfig.MaxRunDuration, err = strconv.ParseInt(os.Getenv("MAX_SIMULATION_DURATION"), 10, 64)
	if err != nil {
		baseLogger.Error("Error parsing MAX_SIMULATION_DURATION", "error", err)
		return
	}
	instanceConfig.Subnet = os.Getenv("SUBNET")
	instanceConfig.DiskSize, err = strconv.ParseInt(os.Getenv("DISK_SIZE"), 10, 64)
	if err != nil {
		baseLogger.Error("Error parsing DISK_SIZE", "error", err)
		return
	}
	instanceConfig.SimulationBucket = os.Getenv("SIMULATION_BUCKET")
	instanceConfig.FinishFunctionURL = os.Getenv("FINISH_FUNCTION_URL")
	if instanceConfig.FinishFunctionURL == "" {
		baseLogger.Error("FINISH_FUNCTION_URL environment variable not set.")
		return
	}

	ctx := context.Background()

	fsClient, err := firestore.NewClient(ctx, projectID, databaseID, baseLogger)
	if err != nil {
		baseLogger.Error("Error creating Firestore client", "error", err)
		return
	}
	defer fsClient.Close()

	cClient, err := compute.NewClient(ctx, baseLogger)
	if err != nil {
		baseLogger.Error("Error creating Compute client", "error", err)
		return
	}
	defer cClient.Close()

	psClient, err := pubsub.NewClient(ctx, projectID, baseLogger)
	if err != nil {
		baseLogger.Error("Error creating PubSub client", "error", err)
		return
	}
	defer psClient.Close()

	scheduler := NewScheduler(fsClient, cClient, psClient, baseLogger)
	err = scheduler.ScheduleSimulation(ctx, projectID, zone, databaseID, maxRunningVMs, subscriptionId, instanceConfig)
	if err != nil {
		baseLogger.Error("Error scheduling simulation", "error", err)
		return
	}
}
