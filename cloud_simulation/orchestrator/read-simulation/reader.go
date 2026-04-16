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
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc/status"

	"simulator.code/simulation-reader/firestore"
)


type firestoreClient interface {
	GetSimulation(ctx context.Context, id string) (*firestore.Simulation, error)
	GetCounter(ctx context.Context) (int, error)
	ListSimulations(ctx context.Context, status, owner, sortBy, sortOrder string, pageSize int, pageToken string) ([]*firestore.Simulation, string, error)
	Close() error
}

type reader struct {
	firestoreClient firestoreClient
	logger          *slog.Logger
	maxRunningVMs   int
	mux             *http.ServeMux
}

func NewReader(fs firestoreClient, logger *slog.Logger, maxRunningVMs int) *reader {
	s := &reader{
		firestoreClient: fs,
		logger:          logger,
		maxRunningVMs:   maxRunningVMs,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /config/max-running-vms", s.handleMaxRunningVMs)
	mux.HandleFunc("GET /simulations", s.handleSimulationsList) // List endpoint
	mux.HandleFunc("GET /simulations/", s.handleSimulationsGet) // Get by ID endpoint
	mux.HandleFunc("GET /simulations/running/count", s.handleRunningSimulationsCount)

	s.mux = mux
	return s
}

func (s *reader) getSimulation(w http.ResponseWriter, r *http.Request, id string) {
	sim, err := s.firestoreClient.GetSimulation(r.Context(), id)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code().String() == "NotFound" {
			s.logger.Info("simulation not found", "id", id)
			http.NotFound(w, r)
			return
		}
		s.logger.Error("failed to get simulation", "id", id, "error", err)
		http.Error(w, "failed to retrieve simulation", http.StatusInternalServerError)
		return
	}
	respondWithJSON(w, http.StatusOK, sim)
}

func (s *reader) handleRunningSimulationsCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method is allowed", http.StatusMethodNotAllowed)
		return
	}
	s.getRunningSimulationsCount(w, r)
}

func (s *reader) getRunningSimulationsCount(w http.ResponseWriter, r *http.Request) {
	count, err := s.firestoreClient.GetCounter(r.Context())
	if err != nil {
		s.logger.Error("failed to get running simulations count", "error", err)
		http.Error(w, "failed to retrieve running simulations count", http.StatusInternalServerError)
		return
	}

	response := struct {
		Count int `json:"count"`
	}{
		Count: count,
	}
	respondWithJSON(w, http.StatusOK, response)
}

func (s *reader) handleMaxRunningVMs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method is allowed", http.StatusMethodNotAllowed)
		return
	}
	s.getMaxRunningVMs(w, r)
}

func (s *reader) getMaxRunningVMs(w http.ResponseWriter, r *http.Request) {
	response := struct {
		MaxRunningVMs int `json:"max_running_vms"`
	}{MaxRunningVMs: s.maxRunningVMs}
	respondWithJSON(w, http.StatusOK, response)
}

func (s *reader) listSimulations(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	owner := r.URL.Query().Get("owner")
	sortBy := r.URL.Query().Get("sort_by")
	sortOrder := r.URL.Query().Get("sort_order")
	pageToken := r.URL.Query().Get("page_token")
	pageSizeStr := r.URL.Query().Get("page_size")

	allowedSortBy := map[string]bool{
		"received_at":       true,
		"status_updated_at": true,
		"started_at":        true,
	}
	if sortBy != "" && !allowedSortBy[sortBy] {
		msg := "Invalid 'sort_by' parameter. Allowed values are: " +
			"received_at, status_updated_at, started_at."
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Default sorting if not provided
	if sortBy == "" {
		sortBy = "received_at"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Validate sortOrder
	if sortOrder != "asc" && sortOrder != "desc" {
		http.Error(w, "Invalid 'sort_order' parameter. Allowed values are: asc, desc.", http.StatusBadRequest)
		return
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize <= 0 {
		pageSize = 20 // default page size
	}

	sims, nextPageToken, err := s.firestoreClient.ListSimulations(r.Context(), status, owner, sortBy, sortOrder, pageSize, pageToken)
	if err != nil {
		// The Firestore client library will often return an error with a URL to create a missing composite index.
		// Logging the full error is crucial for debugging.
		s.logger.Warn("ListSimulations failed. This might be due to a missing Firestore composite index. Check the 'error' field in this log for a URL to create the required index.",
			"status", status, "owner", owner, "sortBy", sortBy, "error", err)
		http.Error(w, "failed to list simulations", http.StatusInternalServerError)
		return
	}

	// Ensure we return an empty slice instead of null if no simulations are found.
	if sims == nil {
		sims = []*firestore.Simulation{}
	}

	response := struct {
		Simulations   []*firestore.Simulation `json:"simulations"`
		NextPageToken string                  `json:"next_page_token,omitempty"`
	}{
		Simulations:   sims,
		NextPageToken: nextPageToken,
	}

	respondWithJSON(w, http.StatusOK, response)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "Error marshalling response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// HandleReadRequest is the entry point for the Cloud Function.
func HandleReadRequest(w http.ResponseWriter, r *http.Request) {
	baseLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With(
		"function_name", "read-simulation",
		"product", "cloud-telemetry-simulator",
	)

	projectID := os.Getenv("GCP_PROJECT")
	databaseID := os.Getenv("FIRESTORE_DATABASE")
	maxRunningVMs, err := strconv.Atoi(os.Getenv("MAX_VM_COUNT"))
	if err != nil {
		baseLogger.Error("Error parsing MAX_VM_COUNT", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx := context.Background()
	fsClient, err := firestore.NewClient(ctx, projectID, databaseID, baseLogger)
	if err != nil {
		baseLogger.Error("Error creating Firestore client", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer fsClient.Close()

	s := NewReader(fsClient, baseLogger, maxRunningVMs)
	s.ServeHTTP(w, r)
}

// handleSimulationsGet handles requests for a single simulation, e.g., GET /simulations/{id}.
func (s *reader) handleSimulationsGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/simulations/")
	if id == "" {
		// This can happen if the request is to "/simulations/" with a trailing slash.
		// http.ServeMux will redirect to the path without the slash if a handler exists for it.
		http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusMovedPermanently)
		return
	}
	s.getSimulation(w, r, id)
}

// handleSimulationsList handles requests to list all simulations, i.e., GET /simulations.
func (s *reader) handleSimulationsList(w http.ResponseWriter, r *http.Request) {
	s.listSimulations(w, r)
}

func (s *reader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("Handling read request", "method", r.Method, "path", r.URL.Path)
	s.mux.ServeHTTP(w, r)
}
