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

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/firestore"
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
	CounterField                           = "counter"
)

type CounterDoc struct {
	Counter int `firestore:"counter"`
}

type Client interface {
	GetCounter(ctx context.Context) (int, error)
	UpdateCounter(ctx context.Context) error
	GetSimulation(ctx context.Context, id string) (*SimulationDoc, error)
	GetStaleSimulations(ctx context.Context, ageInHours int) ([]*SimulationDoc, error)
	UpdateStatus(ctx context.Context, id string, status SimulationStatus) error
	Close() error
}
type client struct {
	client *firestore.Client
	logger *slog.Logger
}

func NewClient(ctx context.Context, projectID string, databaseID string, logger *slog.Logger) (Client, error) {
	firestoreClient, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client: %w", err)
	}
	return &client{client: firestoreClient, logger: logger}, nil
}

type SimulationDoc struct {
	DocumentID        string
	ID                string           `firestore:"id"`
	Status            SimulationStatus `firestore:"status"`
	InstanceID        string           `firestore:"instance_id"`
	BuildID           string           `firestore:"build_id"`
	FilePath          string           `firestore:"file_path"`
	InstanceType      string           `firestore:"instance_type"`
	Owner             string           `firestore:"owner"`
	MaxSimulationTime int              `firestore:"max_simulation_time"`
	MaxReportCount    int              `firestore:"max_report_count"`
}

func (c client) GetSimulation(ctx context.Context, id string) (*SimulationDoc, error) {
	query := c.client.Collection(SimulationsCollection).Where("id", "==", id).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil || len(docs) == 0 {
		return nil, err
	}

	var simulation SimulationDoc
	if err := docs[0].DataTo(&simulation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal simulation document data: %w", err)
	}
	simulation.DocumentID = docs[0].Ref.ID

	return &simulation, nil
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

// UpdateCounter atomically decrements the 'counter' field in the 'running-vms' collection.
// This prevents race conditions that can occur with read-modify-write cycles.
func (c client) UpdateCounter(ctx context.Context) error {
	_, err := c.client.Collection(RunningVMsCollection).Doc(CounterDocument).Update(ctx, []firestore.Update{
		{Path: CounterField, Value: firestore.Increment(-1)},
	})
	if err != nil {
		return fmt.Errorf("failed to atomically decrement counter: %w", err)
	}
	c.logger.Info("Counter decremented successfully in Firestore.")
	return nil
}

func (c client) UpdateStatus(ctx context.Context, id string, status SimulationStatus) error {
	_, err := c.client.Collection(SimulationsCollection).Doc(id).Set(ctx, map[string]interface{}{
		"status":            status,
		"status_updated_at": firestore.ServerTimestamp,
	}, firestore.MergeAll)
	return err
}

func (c client) GetStaleSimulations(ctx context.Context, ageInHours int) ([]*SimulationDoc, error) {
	// We need the current time to query for old documents.
	// time.Now() is sufficient for this purpose, both locally and on GCP.
	now := time.Now()

	if !metadata.OnGCE() {
		c.logger.Warn("Not running on GCE, using local time. This is fine for local testing.")
	}

	var simulations []*SimulationDoc
	query := c.client.Collection(SimulationsCollection).
		Where("status", "in", []any{StatusRunning, StatusScheduled}).
		Where("status_updated_at", "<=", now.Add(-time.Duration(ageInHours)*time.Hour))

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to query for stale simulations: %w", err)
	}

	for _, doc := range docs {
		var sim SimulationDoc
		doc.DataTo(&sim)
		sim.DocumentID = doc.Ref.ID
		simulations = append(simulations, &sim)
	}
	return simulations, nil
}

func (c client) Close() error {
	return c.client.Close()
}
