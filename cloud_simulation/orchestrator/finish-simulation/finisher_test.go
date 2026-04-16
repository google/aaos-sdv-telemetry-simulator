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
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"
	firestore "simulator.code/finish-simulation/firestore"
	mock_main "simulator.code/finish-simulation/mock"
)

func TestFinisher_FinishSimulation(t *testing.T) {
	ctx := context.Background()
	reqData := FinishRequest{
		ProjectID:  "test-project",
		Zone:       "test-zone",
		InstanceID: "test-instance",
		DocumentID: "doc-123",
		Status:     "completed",
	}

	mockInstance := &computepb.Instance{
		Metadata: &computepb.Metadata{
			Items: []*computepb.Items{
				{
					Key:   proto.String("document_id"),
					Value: proto.String(reqData.DocumentID),
				},
			},
		},
	}

	tests := []struct {
		name           string
		setupMocks     func(mockFirestore *mock_main.MockfirestoreClient, mockCompute *mock_main.MockcomputeClient)
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "successful finish",
			setupMocks: func(mockFirestore *mock_main.MockfirestoreClient, mockCompute *mock_main.MockcomputeClient) {
				mockCompute.EXPECT().GetInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(mockInstance, nil)
				mockFirestore.EXPECT().GetSimulation(ctx, reqData.DocumentID).Return(&firestore.SimulationDoc{Status: firestore.StatusRunning}, nil)
				mockFirestore.EXPECT().UpdateStatus(ctx, reqData.DocumentID, firestore.StatusCompleted).Return(nil)
				mockFirestore.EXPECT().DecrementRunningVMsCounter(ctx).Return(nil)
				mockCompute.EXPECT().DeleteInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "identity verification fails due to mismatch",
			setupMocks: func(mockFirestore *mock_main.MockfirestoreClient, mockCompute *mock_main.MockcomputeClient) {
				mismatchedInstance := &computepb.Instance{
					Metadata: &computepb.Metadata{
						Items: []*computepb.Items{
							{
								Key:   proto.String("document_id"),
								Value: proto.String("different-doc-id"),
							},
						},
					},
				}
				mockCompute.EXPECT().GetInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(mismatchedInstance, nil)
			},
			wantErr:        true,
			wantErrContain: "document ID mismatch",
		},
		{
			name: "instance not found during verification",
			setupMocks: func(mockFirestore *mock_main.MockfirestoreClient, mockCompute *mock_main.MockcomputeClient) {
				notFoundErr := &googleapi.Error{Code: http.StatusNotFound}
				mockCompute.EXPECT().GetInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(nil, notFoundErr)
				// Cleanup should still proceed
				mockFirestore.EXPECT().GetSimulation(ctx, reqData.DocumentID).Return(&firestore.SimulationDoc{Status: firestore.StatusRunning}, nil)
				mockFirestore.EXPECT().UpdateStatus(ctx, reqData.DocumentID, firestore.StatusCompleted).Return(nil)
				mockFirestore.EXPECT().DecrementRunningVMsCounter(ctx).Return(nil)
				mockCompute.EXPECT().DeleteInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "get instance fails with non-404 error",
			setupMocks: func(mockFirestore *mock_main.MockfirestoreClient, mockCompute *mock_main.MockcomputeClient) {
				genericErr := errors.New("generic compute error")
				mockCompute.EXPECT().GetInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(nil, genericErr)
			},
			wantErr:        true,
			wantErrContain: "failed to get instance details for verification",
		},
		{
			name: "delete instance fails",
			setupMocks: func(mockFirestore *mock_main.MockfirestoreClient, mockCompute *mock_main.MockcomputeClient) {
				deleteErr := errors.New("failed to delete")
				mockCompute.EXPECT().GetInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(mockInstance, nil)
				mockFirestore.EXPECT().GetSimulation(ctx, reqData.DocumentID).Return(&firestore.SimulationDoc{Status: firestore.StatusRunning}, nil)
				mockFirestore.EXPECT().UpdateStatus(ctx, reqData.DocumentID, firestore.StatusCompleted).Return(nil)
				mockFirestore.EXPECT().DecrementRunningVMsCounter(ctx).Return(nil)
				mockCompute.EXPECT().DeleteInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(deleteErr)
			},
			wantErr:        true,
			wantErrContain: "failed to delete instance",
		},
		{
			name: "update status fails but cleanup continues",
			setupMocks: func(mockFirestore *mock_main.MockfirestoreClient, mockCompute *mock_main.MockcomputeClient) {
				statusErr := errors.New("failed to update status")
				mockCompute.EXPECT().GetInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(mockInstance, nil)
				mockFirestore.EXPECT().GetSimulation(ctx, reqData.DocumentID).Return(&firestore.SimulationDoc{Status: firestore.StatusRunning}, nil)
				mockFirestore.EXPECT().UpdateStatus(ctx, reqData.DocumentID, firestore.StatusCompleted).Return(statusErr)
				// Other steps should still run
				mockFirestore.EXPECT().DecrementRunningVMsCounter(ctx).Return(nil)
				mockCompute.EXPECT().DeleteInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(nil)
			},
			wantErr: false, // Error from UpdateStatus should be logged but not returned
		},
		{
			name: "update counter fails but cleanup continues",
			setupMocks: func(mockFirestore *mock_main.MockfirestoreClient, mockCompute *mock_main.MockcomputeClient) {
				counterErr := errors.New("failed to update counter")
				mockCompute.EXPECT().GetInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(mockInstance, nil)
				mockFirestore.EXPECT().GetSimulation(ctx, reqData.DocumentID).Return(&firestore.SimulationDoc{Status: firestore.StatusRunning}, nil)
				mockFirestore.EXPECT().UpdateStatus(ctx, reqData.DocumentID, firestore.StatusCompleted).Return(nil)
				mockFirestore.EXPECT().DecrementRunningVMsCounter(ctx).Return(counterErr)
				// Delete should still run
				mockCompute.EXPECT().DeleteInstance(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID).Return(nil)
			},
			wantErr: false, // Error from DecrementRunningVMsCounter should be logged but not returned
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFirestore := mock_main.NewMockfirestoreClient(ctrl)
			mockCompute := mock_main.NewMockcomputeClient(ctrl)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			f := NewFinisher(mockFirestore, mockCompute, logger)

			tt.setupMocks(mockFirestore, mockCompute)

			err := f.FinishSimulation(ctx, reqData.ProjectID, reqData.Zone, reqData.InstanceID, reqData.DocumentID, firestore.StatusCompleted)

			if tt.wantErr {
				if err == nil {
					t.Errorf("FinishSimulation() = nil, want error")
				} else if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("FinishSimulation() error = %v, want error containing %q", err, tt.wantErrContain)
				}
			} else if err != nil {
				t.Errorf("FinishSimulation() = %v, want nil", err)
			}
		})
	}
}
