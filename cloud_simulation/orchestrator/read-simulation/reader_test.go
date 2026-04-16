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

package reader

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"simulator.code/simulation-reader/firestore"
	mock_firestore "simulator.code/simulation-reader/mock"
)

func TestGetSimulation_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodGet, "/simulations/sim-123", nil)
	rr := httptest.NewRecorder()

	expectedSim := &firestore.Simulation{ID: "sim-123", Status: "running"}
	mockFirestore.EXPECT().GetSimulation(gomock.Any(), "sim-123").Return(expectedSim, nil)

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var actualSim firestore.Simulation
	err := json.Unmarshal(rr.Body.Bytes(), &actualSim)
	require.NoError(t, err)
	assert.Equal(t, *expectedSim, actualSim)
}

func TestListSimulations_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodGet, "/simulations?status=running&page_size=10", nil)
	rr := httptest.NewRecorder()

	expectedSims := []*firestore.Simulation{
		{ID: "sim-123", Status: "running"},
		{ID: "sim-456", Status: "running"},
	}
	mockFirestore.EXPECT().ListSimulations(gomock.Any(), "running", "", "received_at", "desc", 10, "").Return(expectedSims, "next-token", nil)

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response struct {
		Simulations   []*firestore.Simulation `json:"simulations"`
		NextPageToken string                  `json:"next_page_token"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, expectedSims, response.Simulations)
	assert.Equal(t, "next-token", response.NextPageToken)
}

func TestListSimulations_SuccessNoParams(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodGet, "/simulations", nil)
	rr := httptest.NewRecorder()

	expectedSims := []*firestore.Simulation{
		{ID: "sim-123", Status: "running", Owner: "user1@example.com"},
		{ID: "sim-456", Status: "completed", Owner: "user2@example.com"},
	}
	// Expect a call with default sorting and empty filters for status and owner.
	mockFirestore.EXPECT().ListSimulations(gomock.Any(), "", "", "received_at", "desc", 20, "").Return(expectedSims, "", nil)

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response struct {
		Simulations []*firestore.Simulation `json:"simulations"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, expectedSims, response.Simulations)
}

func TestListSimulations_FilterAndSort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodGet, "/simulations?status=completed&owner=testuser&sort_by=status_updated_at&sort_order=asc&page_size=5", nil)
	rr := httptest.NewRecorder()

	expectedSims := []*firestore.Simulation{
		{ID: "sim-789", Status: "completed", Owner: "testuser"},
		{ID: "sim-012", Status: "completed", Owner: "testuser"},
	}
	mockFirestore.EXPECT().ListSimulations(gomock.Any(), "completed", "testuser", "status_updated_at", "asc", 5, "").Return(expectedSims, "", nil)

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response struct {
		Simulations   []*firestore.Simulation `json:"simulations"`
		NextPageToken string                  `json:"next_page_token"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, expectedSims, response.Simulations)
	assert.Empty(t, response.NextPageToken)
}

func TestListSimulations_InvalidSortBy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodGet, "/simulations?sort_by=invalid_field", nil)
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Invalid 'sort_by' parameter. Allowed values are: received_at, status_updated_at, started_at.")
}

func TestListSimulations_InvalidSortOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodGet, "/simulations?sort_order=invalid", nil)
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Invalid 'sort_order' parameter. Allowed values are: asc, desc.")
}

func TestListSimulations_InvalidPageToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	mockFirestore.EXPECT().ListSimulations(gomock.Any(), "", "", "received_at", "desc", 20, "non-existent-token").Return(nil, "", assert.AnError) // Simulate Firestore error

	req := httptest.NewRequest(http.MethodGet, "/simulations?page_token=non-existent-token", nil)
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code) // Or Bad Request if the error message is specific
}

func TestGetRunningSimulationsCount_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodGet, "/simulations/running/count", nil)
	rr := httptest.NewRecorder()

	mockFirestore.EXPECT().GetCounter(gomock.Any()).Return(42, nil)

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response struct {
		Count int `json:"count"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 42, response.Count)
}

func TestGetMaxRunningVMs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 150)

	req := httptest.NewRequest(http.MethodGet, "/config/max-running-vms", nil)
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response struct {
		MaxRunningVMs int `json:"max_running_vms"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 150, response.MaxRunningVMs)
}

func TestServeHTTP_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodGet, "/invalid/path", nil)
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestServeHTTP_MethodNotAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFirestore := mock_firestore.NewMockfirestoreClient(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewReader(mockFirestore, logger, 100)

	req := httptest.NewRequest(http.MethodPost, "/simulations", nil)
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}
