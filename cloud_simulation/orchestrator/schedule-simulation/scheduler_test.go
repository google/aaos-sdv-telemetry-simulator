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
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"simulator.code/simulation-scheduler/compute"
	mock_scheduler "simulator.code/simulation-scheduler/mock"
	"simulator.code/simulation-scheduler/pubsub"
)

func TestScheduleSimulation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_scheduler.NewMockfirestoreClient(ctrl)
	mockCompute := mock_scheduler.NewMockcomputeClient(ctrl)
	mockPubsub := mock_scheduler.NewMockpubsubClient(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := NewScheduler(mockFirestore, mockCompute, mockPubsub, logger)

	ctx := context.Background()
	projectID := "test-project"
	zone := "test-zone"
	databaseID := "test-database"
	maxRunningVMs := 5
	subscriptionID := "test-subscription"
	instanceConfig := compute.InstanceConfig{
		FinishFunctionURL: "http://finish.url",
	}

	mockFirestore.EXPECT().CheckCounter(ctx, maxRunningVMs).Return(true, nil)
	expectedMessage := pubsub.AvroRecord{
		ID:                "sim-123",
		BuildID:           "build-456",
		InstanceType:      "n1-standard-4",
		MaxSimulationTime: 120,
		MaxReportCount:    10,
	}
	mockPubsub.EXPECT().PullMessage(ctx, subscriptionID).Return(&expectedMessage, nil)

	expectedDocID := "doc-abc-123"
	mockFirestore.EXPECT().GetSimulationDocID(ctx, expectedMessage.ID).Return(expectedDocID, nil)

	mockFirestore.EXPECT().IncrementCounter(ctx, maxRunningVMs).Return(nil)

	expectedInstanceID := "test-instance-123"
	expectedIP := "192.168.1.1"
	expectedConfig := compute.InstanceConfig{
		SimulationID:      "sim-123",
		DockerImage:       "build-456",
		InstanceType:      "n1-standard-4",
		DocumentID:        expectedDocID,
		MaxSimulationTime: 120,
		MaxReportCount:    10,
		FinishFunctionURL: "http://finish.url",
	}
	mockCompute.EXPECT().CreateInstance(ctx, projectID, zone, databaseID, expectedConfig).Return(expectedInstanceID, expectedIP, nil)

	mockFirestore.EXPECT().SimulationUpdate(ctx, "sim-123", expectedInstanceID, expectedIP).Return(nil)

	err := scheduler.ScheduleSimulation(ctx, projectID, zone, databaseID, maxRunningVMs, subscriptionID, instanceConfig)

	assert.NoError(t, err)
}

func TestScheduleSimulation_maxRunningVMsReached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_scheduler.NewMockfirestoreClient(ctrl)
	mockCompute := mock_scheduler.NewMockcomputeClient(ctrl)
	mockPubsub := mock_scheduler.NewMockpubsubClient(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := NewScheduler(mockFirestore, mockCompute, mockPubsub, logger)

	ctx := context.Background()
	projectID := "test-project"
	zone := "test-zone"
	databaseID := "test-database"
	maxRunningVMs := 5
	subscriptionID := "test-subscription"
	instanceConfig := compute.InstanceConfig{
		FinishFunctionURL: "http://finish.url",
	}

	mockFirestore.EXPECT().CheckCounter(ctx, maxRunningVMs).Return(false, nil)
	err := scheduler.ScheduleSimulation(ctx, projectID, zone, databaseID, maxRunningVMs, subscriptionID, instanceConfig)

	assert.NoError(t, err)
}

func TestScheduleSimulation_ErrorPullingMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_scheduler.NewMockfirestoreClient(ctrl)
	mockCompute := mock_scheduler.NewMockcomputeClient(ctrl)
	mockPubsub := mock_scheduler.NewMockpubsubClient(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := NewScheduler(mockFirestore, mockCompute, mockPubsub, logger)

	ctx := context.Background()
	projectID := "test-project"
	zone := "test-zone"
	databaseID := "test-database"
	maxRunningVMs := 5
	subscriptionID := "test-subscription"
	instanceConfig := compute.InstanceConfig{
		FinishFunctionURL: "http://finish.url",
	}

	mockFirestore.EXPECT().CheckCounter(ctx, maxRunningVMs).Return(true, nil)
	mockPubsub.EXPECT().PullMessage(ctx, subscriptionID).Return(&pubsub.AvroRecord{}, assert.AnError)

	err := scheduler.ScheduleSimulation(ctx, projectID, zone, databaseID, maxRunningVMs, subscriptionID, instanceConfig)
	assert.Error(t, err)
}

func TestScheduleSimulation_ErrorCreatingInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_scheduler.NewMockfirestoreClient(ctrl)
	mockCompute := mock_scheduler.NewMockcomputeClient(ctrl)
	mockPubsub := mock_scheduler.NewMockpubsubClient(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := NewScheduler(mockFirestore, mockCompute, mockPubsub, logger)

	ctx := context.Background()
	projectID := "test-project"
	databaseID := "test-database"
	zone := "test-zone"
	maxRunningVMs := 5
	subscriptionID := "test-subscription"
	instanceConfig := compute.InstanceConfig{
		FinishFunctionURL: "http://finish.url",
	}

	mockFirestore.EXPECT().CheckCounter(ctx, maxRunningVMs).Return(true, nil)

	expectedMessage := pubsub.AvroRecord{
		ID:                "sim-123",
		BuildID:           "build-456",
		InstanceType:      "n1-standard-4",
		MaxSimulationTime: 120,
		MaxReportCount:    10,
	}
	mockPubsub.EXPECT().PullMessage(ctx, subscriptionID).Return(&expectedMessage, nil)

	expectedDocID := "doc-abc-123"
	mockFirestore.EXPECT().GetSimulationDocID(ctx, expectedMessage.ID).Return(expectedDocID, nil)

	expectedConfig := compute.InstanceConfig{
		SimulationID:      "sim-123",
		DockerImage:       "build-456",
		InstanceType:      "n1-standard-4",
		DocumentID:        expectedDocID,
		MaxSimulationTime: 120,
		MaxReportCount:    10,
		FinishFunctionURL: "http://finish.url",
	}

	mockFirestore.EXPECT().IncrementCounter(ctx, maxRunningVMs).Return(nil)

	mockCompute.EXPECT().CreateInstance(ctx, projectID, zone, databaseID, expectedConfig).Return("", "", assert.AnError)

	mockFirestore.EXPECT().DecrementCounter(ctx).Return(nil)

	err := scheduler.ScheduleSimulation(ctx, projectID, zone, databaseID, maxRunningVMs, subscriptionID, instanceConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Error creating instance")
}

func TestScheduleSimulation_ErrorGettingDocID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_scheduler.NewMockfirestoreClient(ctrl)
	mockCompute := mock_scheduler.NewMockcomputeClient(ctrl)
	mockPubsub := mock_scheduler.NewMockpubsubClient(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := NewScheduler(mockFirestore, mockCompute, mockPubsub, logger)

	ctx := context.Background()
	projectID := "test-project"
	zone := "test-zone"
	databaseID := "test-database"
	maxRunningVMs := 5
	subscriptionID := "test-subscription"
	instanceConfig := compute.InstanceConfig{
		FinishFunctionURL: "http://finish.url",
	}

	mockFirestore.EXPECT().CheckCounter(ctx, maxRunningVMs).Return(true, nil)
	expectedMessage := pubsub.AvroRecord{
		ID: "sim-123",
	}
	mockPubsub.EXPECT().PullMessage(ctx, subscriptionID).Return(&expectedMessage, nil)

	mockFirestore.EXPECT().GetSimulationDocID(ctx, expectedMessage.ID).Return("", assert.AnError)

	err := scheduler.ScheduleSimulation(ctx, projectID, zone, databaseID, maxRunningVMs, subscriptionID, instanceConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get document ID")
}
