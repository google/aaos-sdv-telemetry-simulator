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
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	firestore "simulator.code/delete-simulation/firestore"
	mock_main "simulator.code/delete-simulation/mock"
)

func TestCancelRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_main.NewMockfirestoreClient(ctrl)
	mockCompute := mock_main.NewMockcomputeClient(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	canceller := NewCanceller(mockFirestore, mockCompute, logger)

	ctx := context.Background()
	simulationID := "test-simulation"
	documentID := "xyz"
	projectID := "test-project"
	zone := "test-zone"

	t.Run("Cancel running simulation", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(&firestore.SimulationDoc{
			Status:     firestore.StatusRunning,
			DocumentID: documentID,
		}, nil)
		mockFirestore.EXPECT().UpdateStatus(ctx, documentID, firestore.StatusCancelled).Return(nil)

		err := canceller.CancelRequest(ctx, simulationID, projectID, zone)
		assert.NoError(t, err)
	})

	t.Run("Cancel scheduled simulation", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(&firestore.SimulationDoc{
			Status:     firestore.StatusScheduled,
			DocumentID: documentID,
		}, nil)
		mockFirestore.EXPECT().UpdateStatus(ctx, documentID, firestore.StatusCancelled).Return(nil)

		err := canceller.CancelRequest(ctx, simulationID, projectID, zone)
		assert.NoError(t, err)
	})

	t.Run("Cannot cancel completed simulation", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(&firestore.SimulationDoc{
			Status:     firestore.StatusCompleted,
			DocumentID: documentID,
		}, nil)

		err := canceller.CancelRequest(ctx, simulationID, projectID, zone)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Simulation cannot be cancelled")
	})

	t.Run("Simulation not found", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(nil, assert.AnError)

		err := canceller.CancelRequest(ctx, simulationID, projectID, zone)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error getting simulation")
	})

	t.Run("Fail to update status", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(&firestore.SimulationDoc{
			Status:     firestore.StatusRunning,
			DocumentID: documentID,
		}, nil)
		mockFirestore.EXPECT().UpdateStatus(ctx, documentID, firestore.StatusCancelled).Return(assert.AnError)

		err := canceller.CancelRequest(ctx, simulationID, projectID, zone)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Failed to update status")
	})
}

func TestCleanupAfterCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_main.NewMockfirestoreClient(ctrl)
	mockCompute := mock_main.NewMockcomputeClient(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	canceller := NewCanceller(mockFirestore, mockCompute, logger)

	ctx := context.Background()
	simulationID := "test-simulation"
	instanceID := "test-instance"
	projectID := "test-project"
	zone := "test-zone"

	t.Run("Successful cleanup", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(&firestore.SimulationDoc{
			InstanceID: instanceID,
		}, nil)
		mockCompute.EXPECT().DeleteInstance(ctx, projectID, zone, instanceID).Return(nil)
		mockFirestore.EXPECT().UpdateCounter(ctx).Return(nil)

		err := canceller.CleanupAfterCancellation(ctx, simulationID, projectID, zone)
		assert.NoError(t, err)
	})

	t.Run("Failed to get simulation", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(nil, assert.AnError)

		err := canceller.CleanupAfterCancellation(ctx, simulationID, projectID, zone)
		// CleanupAfterCancellation returns an error if GetSimulation returns an error.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error getting simulation")
	})

	t.Run("Failed to delete instance", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(&firestore.SimulationDoc{
			InstanceID: instanceID,
		}, nil)
		mockCompute.EXPECT().DeleteInstance(ctx, projectID, zone, instanceID).Return(errors.New("compute error"))

		err := canceller.CleanupAfterCancellation(ctx, simulationID, projectID, zone)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Failed to delete compute instance")
	})

	t.Run("Failed to update counter", func(t *testing.T) {
		mockFirestore.EXPECT().GetSimulation(ctx, simulationID).Return(&firestore.SimulationDoc{
			InstanceID: instanceID,
		}, nil)
		mockCompute.EXPECT().DeleteInstance(ctx, projectID, zone, instanceID).Return(nil)
		mockFirestore.EXPECT().UpdateCounter(ctx).Return(assert.AnError).Times(3)

		err := canceller.CleanupAfterCancellation(ctx, simulationID, projectID, zone)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Max retries reached for updating counter")
	})
}

func TestCleanup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_main.NewMockfirestoreClient(ctrl)
	mockCompute := mock_main.NewMockcomputeClient(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	canceller := NewCanceller(mockFirestore, mockCompute, logger)

	ctx := context.Background()
	simulationID := "test-simulation"
	instanceID := "test-instance"
	projectID := "test-project"
	zone := "test-zone"

	t.Run("Successful cleanup", func(t *testing.T) {
		mockFirestore.EXPECT().GetStaleSimulations(ctx, 24).Return([]*firestore.SimulationDoc{{DocumentID: "stale-doc-id", InstanceID: "stale-instance-id"}}, nil)
		mockCompute.EXPECT().InstanceExists(ctx, projectID, zone, "stale-instance-id").Return(false, nil)
		mockFirestore.EXPECT().UpdateStatus(ctx, "stale-doc-id", firestore.StatusCancelled).Return(nil)
		mockCompute.EXPECT().QueryInstances(ctx, projectID, zone, 24).Return([]string{instanceID}, nil)
		mockFirestore.EXPECT().GetSimulation(ctx, instanceID).Return(&firestore.SimulationDoc{
			DocumentID: simulationID,
			InstanceID: instanceID,
		}, nil)
		mockCompute.EXPECT().DeleteInstance(ctx, projectID, zone, instanceID).Return(nil)
		mockFirestore.EXPECT().UpdateStatus(ctx, simulationID, firestore.StatusCancelled).Return(nil)
		err := canceller.Cleanup(ctx, projectID, zone)
		assert.NoError(t, err)
	})

	t.Run("Failed to query instances", func(t *testing.T) {
		mockCompute.EXPECT().QueryInstances(ctx, projectID, zone, 24).Return(nil, errors.New("query error"))
		mockFirestore.EXPECT().GetStaleSimulations(ctx, 24).Return(nil, nil)

		err := canceller.Cleanup(ctx, projectID, zone)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Failed querying for running simulation instances")
	})

	t.Run("Failed to get simulation", func(t *testing.T) {
		mockFirestore.EXPECT().GetStaleSimulations(ctx, 24).Return(nil, nil)
		mockCompute.EXPECT().QueryInstances(ctx, projectID, zone, 24).Return([]string{instanceID}, nil)
		mockFirestore.EXPECT().GetSimulation(ctx, instanceID).Return(nil, errors.New("firestore error"))
		err := canceller.Cleanup(ctx, projectID, zone) // Cleanup logs and continues, returning nil
		assert.NoError(t, err)                         // Expect no error from Cleanup in this scenario
	})

	t.Run("Failed to delete instance", func(t *testing.T) {
		mockFirestore.EXPECT().GetStaleSimulations(ctx, 24).Return(nil, nil)
		mockCompute.EXPECT().QueryInstances(ctx, projectID, zone, 24).Return([]string{instanceID}, nil)
		mockFirestore.EXPECT().GetSimulation(ctx, instanceID).Return(&firestore.SimulationDoc{
			DocumentID: simulationID,
			InstanceID: instanceID,
		}, nil)
		mockCompute.EXPECT().DeleteInstance(ctx, projectID, zone, instanceID).Return(errors.New("compute error"))
		err := canceller.Cleanup(ctx, projectID, zone)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Failed deleting instance")
	})

	t.Run("Failed to update status", func(t *testing.T) {
		mockFirestore.EXPECT().GetStaleSimulations(ctx, 24).Return(nil, nil)
		mockCompute.EXPECT().QueryInstances(ctx, projectID, zone, 24).Return([]string{instanceID}, nil)
		mockFirestore.EXPECT().GetSimulation(ctx, instanceID).Return(&firestore.SimulationDoc{
			DocumentID: simulationID,
			InstanceID: instanceID,
		}, nil)
		mockCompute.EXPECT().DeleteInstance(ctx, projectID, zone, instanceID).Return(nil)
		mockFirestore.EXPECT().UpdateStatus(ctx, simulationID, firestore.StatusCancelled).Return(errors.New("status update error"))

		err := canceller.Cleanup(ctx, projectID, zone)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Failed updating simulation status")
	})

	t.Run("Simulation not found", func(t *testing.T) {
		mockFirestore.EXPECT().GetStaleSimulations(ctx, 24).Return(nil, nil)
		mockCompute.EXPECT().QueryInstances(ctx, projectID, zone, 24).Return([]string{instanceID}, nil)
		mockFirestore.EXPECT().GetSimulation(ctx, instanceID).Return(nil, nil)

		err := canceller.Cleanup(ctx, projectID, zone)
		assert.NoError(t, err) // This should not return an error as per the implementation
	})
}
