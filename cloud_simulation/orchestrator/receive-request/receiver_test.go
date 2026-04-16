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
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	mock_receiver "simulator.code/receive-request/mock"
	"simulator.code/receive-request/pubsub"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

type avroRecordMatcher struct {
	expected pubsub.AvroRecord
}

func (m avroRecordMatcher) Matches(x interface{}) bool {
	actual, ok := x.(pubsub.AvroRecord)
	if !ok {
		return false
	}
	// Don't compare ID as it's random
	return actual.Owner == m.expected.Owner &&
		actual.FilePath == m.expected.FilePath &&
		actual.BuildID == m.expected.BuildID &&
		actual.InstanceType == m.expected.InstanceType &&
		actual.MaxSimulationTime == m.expected.MaxSimulationTime &&
		actual.MaxReportCount == m.expected.MaxReportCount
}

func (m avroRecordMatcher) String() string {
	return fmt.Sprintf("is equal to %v (ignoring ID)", m.expected)
}

func EqAvroRecord(expected pubsub.AvroRecord) gomock.Matcher {
	return avroRecordMatcher{expected: expected}
}

func TestReceiveRequest(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*mock_receiver.MockstorageClient, *mock_receiver.MockfirestoreClient, *mock_receiver.MockpubsubClient)
		requestBody     string
		supportedBuilds string
		expectedStatus  int
		expectedError   string
	}{
		{
			name: "Successful request",
			setupMocks: func(ms *mock_receiver.MockstorageClient, mf *mock_receiver.MockfirestoreClient, mp *mock_receiver.MockpubsubClient) {
				maxSimTime := 60
				maxReportCount := 5
				expectedRecord := pubsub.AvroRecord{
					Owner:             "test-owner",
					FilePath:          "gs://test-bucket/valid/file/path",
					BuildID:           "test-image:test-build",
					InstanceType:      "test-instancetype",
					MaxSimulationTime: maxSimTime,
					MaxReportCount:    maxReportCount,
				}
				ms.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
				ms.EXPECT().CopyToOutputs(gomock.Any(), "gs://test-bucket/valid/file/path", gomock.Any())
				mf.EXPECT().Write(gomock.Any(), EqAvroRecord(expectedRecord)).Return(nil)
				mp.EXPECT().PublishMessage(gomock.Any(), "test-topic", EqAvroRecord(expectedRecord)).Return(nil)
			},
			requestBody:     `{"file_path": "valid/file/path", "owner": "test-owner", "build_id": "test-build", "instance_type": "test-instancetype", "max_simulation_time": 60, "max_report_count": 5}`,
			supportedBuilds: ``,
			expectedStatus:  http.StatusOK,
		},
		{
			name: "Storage validation fails",
			setupMocks: func(ms *mock_receiver.MockstorageClient, mf *mock_receiver.MockfirestoreClient, mp *mock_receiver.MockpubsubClient) {
				ms.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(errors.New("validation failed"))
			},
			requestBody:     `{"file_path": "invalid/file/path", "owner": "test-owner", "build_id": "test-build", "instance_type": "test-instancetype", "max_simulation_time": 60, "max_report_count": 5}`,
			supportedBuilds: ``,
			expectedStatus:  http.StatusBadRequest,
			expectedError:   "Error validating file path: validation failed",
		},
		{
			name: "Firestore write fails",
			setupMocks: func(ms *mock_receiver.MockstorageClient, mf *mock_receiver.MockfirestoreClient, mp *mock_receiver.MockpubsubClient) {
				maxSimTime := 60
				maxReportCount := 5
				expectedRecord := pubsub.AvroRecord{
					Owner:             "test-owner",
					FilePath:          "gs://test-bucket/valid/file/path",
					BuildID:           "test-image:test-build",
					InstanceType:      "test-instancetype",
					MaxSimulationTime: maxSimTime,
					MaxReportCount:    maxReportCount,
				}
				ms.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
				mf.EXPECT().Write(gomock.Any(), EqAvroRecord(expectedRecord)).Return(errors.New("write failed"))
			},
			requestBody:     `{"file_path": "valid/file/path", "owner": "test-owner", "build_id": "test-build", "instance_type": "test-instancetype", "max_simulation_time": 60, "max_report_count": 5}`,
			supportedBuilds: ``,
			expectedStatus:  http.StatusInternalServerError,
			expectedError:   "Error writing to Firestore: write failed",
		},
		{
			name: "Invalid JSON in message",
			setupMocks: func(ms *mock_receiver.MockstorageClient, mf *mock_receiver.MockfirestoreClient, mp *mock_receiver.MockpubsubClient) {
				// No expectations set as the function should return early
			},
			requestBody:     `invalid json`,
			supportedBuilds: ``,
			expectedStatus:  http.StatusBadRequest,
			expectedError:   "Error parsing request body",
		},
		{
			name: "Missing required fields",
			setupMocks: func(ms *mock_receiver.MockstorageClient, mf *mock_receiver.MockfirestoreClient, mp *mock_receiver.MockpubsubClient) {
				// No expectations set as the function should return early
			},
			requestBody:     `{}`,
			supportedBuilds: ``,
			expectedStatus:  http.StatusBadRequest,
			expectedError:   "Missing required field",
		},
		{
			name: "Unsupported build_id",
			setupMocks: func(ms *mock_receiver.MockstorageClient, mf *mock_receiver.MockfirestoreClient, mp *mock_receiver.MockpubsubClient) {
				// No client calls are expected as validation fails early.
			},
			requestBody:     `{"file_path": "valid/file/path", "owner": "test-owner", "build_id": "unsupported-build", "instance_type": "test-instancetype", "max_simulation_time": 60, "max_report_count": 5}`,
			supportedBuilds: `{"supported-build":"sha256:abc", "another-build":"sha256:def"}`,
			expectedStatus:  http.StatusBadRequest,
			expectedError:   `Unsupported build_id: "unsupported-build". Supported builds are: [another-build supported-build]`,
		},
		{
			name: "Successful request with supported build_id",
			setupMocks: func(ms *mock_receiver.MockstorageClient, mf *mock_receiver.MockfirestoreClient, mp *mock_receiver.MockpubsubClient) {
				maxSimTime := 60
				maxReportCount := 5
				expectedRecord := pubsub.AvroRecord{
					Owner:             "test-owner",
					FilePath:          "gs://test-bucket/valid/file/path",
					BuildID:           "test-image@sha256:abc",
					InstanceType:      "test-instancetype",
					MaxSimulationTime: maxSimTime,
					MaxReportCount:    maxReportCount,
				}
				ms.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
				ms.EXPECT().CopyToOutputs(gomock.Any(), "gs://test-bucket/valid/file/path", gomock.Any())
				mf.EXPECT().Write(gomock.Any(), EqAvroRecord(expectedRecord)).Return(nil)
				mp.EXPECT().PublishMessage(gomock.Any(), "test-topic", EqAvroRecord(expectedRecord)).Return(nil)
			},
			requestBody:     `{"file_path": "valid/file/path", "owner": "test-owner", "build_id": "supported-build", "instance_type": "test-instancetype", "max_simulation_time": 60, "max_report_count": 5}`,
			supportedBuilds: `{"supported-build":"sha256:abc"}`,
			expectedStatus:  http.StatusOK,
		},
		{
			name: "Invalid supported_builds JSON",
			setupMocks: func(ms *mock_receiver.MockstorageClient, mf *mock_receiver.MockfirestoreClient, mp *mock_receiver.MockpubsubClient) {
				// No client calls are expected as config parsing fails early.
			},
			requestBody:     `{"file_path": "valid/file/path", "owner": "test-owner", "build_id": "any-build", "instance_type": "test-instancetype", "max_simulation_time": 60, "max_report_count": 5}`,
			supportedBuilds: `invalid-json`,
			expectedStatus:  http.StatusInternalServerError,
			expectedError:   "Internal server error: invalid build configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := mock_receiver.NewMockstorageClient(ctrl)
			mockFirestore := mock_receiver.NewMockfirestoreClient(ctrl)
			mockPubsub := mock_receiver.NewMockpubsubClient(ctrl)

			tt.setupMocks(mockStorage, mockFirestore, mockPubsub)

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			receiver := NewReceiver(mockFirestore, mockStorage, mockPubsub, logger)

			req := httptest.NewRequest(http.MethodPost, "/receive", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			requestId := "test-request-id"
			receiver.ReceiveRequest(requestId, rr, req, "test-topic", "test-image", "gs://test-bucket", tt.supportedBuilds)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedError != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedError)
			}
		})
	}
}
