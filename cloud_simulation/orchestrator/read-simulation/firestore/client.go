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

package firestore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SimulationStatus string

const (
	StatusRunning         SimulationStatus = "running"
	StatusScheduled       SimulationStatus = "Simulation request received"
	StatusCancelled       SimulationStatus = "cancelled"
	StatusCompleted       SimulationStatus = "completed"
	RunningVMsCollection                   = "running-vms"
	SimulationsCollection                  = "records"
	CounterDocument                        = "counter"
)

type CounterDoc struct {
	Counter int `firestore:"counter"`
}

// Simulation represents the data structure of a simulation document in Firestore.
type Simulation struct {
	ID                string           `firestore:"id" json:"id"`
	Owner             string           `firestore:"owner" json:"owner"`
	Status            SimulationStatus `firestore:"status" json:"status"`
	StatusUpdatedAt   time.Time        `firestore:"status_updated_at" json:"status_updated_at"`
	ReceivedAt        time.Time        `firestore:"received_at" json:"received_at"`
	StartedAt         time.Time        `firestore:"started_at,omitempty" json:"started_at,omitempty"`
	EndedAt           time.Time        `firestore:"ended_at,omitempty" json:"ended_at,omitempty"`
	InstanceID        string           `firestore:"instance_id,omitempty" json:"instance_id,omitempty"`
	IP                string           `firestore:"ip,omitempty" json:"ip,omitempty"`
	BuildID           string           `firestore:"build_id" json:"build_id"`
	InstanceType      string           `firestore:"instance_type" json:"instance_type"`
	FilePath          string           `firestore:"file_path" json:"file_path"`
	MaxSimulationTime int              `firestore:"max_simulation_time" json:"max_simulation_time"`
	MaxReportCount    int              `firestore:"max_report_count" json:"max_report_count"`
}

type Client interface {
	GetCounter(ctx context.Context) (int, error)
	GetSimulation(ctx context.Context, id string) (*Simulation, error)
	ListSimulations(ctx context.Context, status, owner, sortBy, sortOrder string, pageSize int, pageToken string) ([]*Simulation, string, error)
	Close() error
}

type client struct {
	client *firestore.Client
	logger *slog.Logger
}

func NewClient(ctx context.Context, projectID string, databaseID string, logger *slog.Logger) (Client, error) {
	firestoreClient, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, err
	}

	return &client{client: firestoreClient, logger: logger}, nil
}

func (c client) GetCounter(ctx context.Context) (int, error) {
	doc, err := c.client.Collection(RunningVMsCollection).Doc(CounterDocument).Get(ctx)
	if err != nil {
		return -1, fmt.Errorf("failed to get counter document: %w", err)
	}

	var counterData CounterDoc
	if err := doc.DataTo(&counterData); err != nil {
		return -1, fmt.Errorf("failed to unmarshal counter data: %w", err)
	}

	return counterData.Counter, nil
}

func (c *client) GetSimulation(ctx context.Context, id string) (*Simulation, error) {
	iter := c.client.Collection(SimulationsCollection).Where("id", "==", id).Limit(1).Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, status.Errorf(codes.NotFound, "simulation with id %s not found", id)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query for simulation: %w", err)
	}

	var sim Simulation
	if err := doc.DataTo(&sim); err != nil {
		return nil, fmt.Errorf("failed to decode simulation data: %w", err)
	}
	return &sim, nil
}

func (c *client) ListSimulations(ctx context.Context, statusFilter, owner, sortBy, sortOrder string, pageSize int, pageToken string) ([]*Simulation, string, error) {
	query := c.client.Collection(SimulationsCollection).Query

	if statusFilter != "" {
		query = query.Where("status", "==", statusFilter)
	}
	if owner != "" {
		query = query.Where("owner", "==", owner)
	}

	// Default sort if not specified
	if sortBy == "" {
		sortBy = "received_at"
		sortOrder = "desc" // Default to descending for received_at
	}

	direction := firestore.Desc
	if sortOrder == "asc" {
		direction = firestore.Asc
	}

	query = query.OrderBy(sortBy, direction)

	if pageToken != "" {
		// To start after a specific document, we need the document snapshot itself.
		// The pageToken should ideally be the ID of the last document from the previous page.
		docRef := c.client.Collection(SimulationsCollection).Doc(pageToken)
		docSnapshot, err := docRef.Get(ctx)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil, "", fmt.Errorf("invalid page token: document with ID %s not found", pageToken)
			}
			return nil, "", fmt.Errorf("invalid page token: %w", err)
		}
		query = query.StartAfter(docSnapshot)
	}

	iter := query.Limit(pageSize).Documents(ctx)
	defer iter.Stop()

	var sims []*Simulation
	var lastDoc *firestore.DocumentSnapshot // Will hold the last document for pagination
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("error iterating over simulations: %w", err)
		}
		lastDoc = doc
		var sim Simulation
		if err := doc.DataTo(&sim); err != nil {
			c.logger.Error("failed to decode simulation data", "doc_id", doc.Ref.ID, "error", err)
			continue // Skip this document and proceed to the next one
		}
		sims = append(sims, &sim)
	}

	nextPageToken := ""
	if lastDoc != nil {
		nextPageToken = lastDoc.Ref.ID
	}
	return sims, nextPageToken, nil
}

func (f client) Close() error {
	err := f.client.Close()
	return err
}
