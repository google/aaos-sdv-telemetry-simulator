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
	DecrementRunningVMsCounter(ctx context.Context) error
	GetSimulation(ctx context.Context, id string) (*SimulationDoc, error)
	UpdateStatus(ctx context.Context, id string, status SimulationStatus) error
	Close() error
}
type client struct {
	client *firestore.Client
}

func NewClient(ctx context.Context, projectID string, databaseID string) (Client, error) {
	firestoreClient, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client: %w", err)
	}
	return &client{client: firestoreClient}, nil
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
	doc, err := c.client.Collection(SimulationsCollection).Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}
	var simulation SimulationDoc
	if err := doc.DataTo(&simulation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal simulation document data: %w", err)
	}
	simulation.DocumentID = doc.Ref.ID

	return &simulation, nil
}

func (f client) DecrementRunningVMsCounter(ctx context.Context) error {
	_, err := f.client.Collection(RunningVMsCollection).Doc(CounterDocument).Update(ctx, []firestore.Update{
		{Path: CounterField, Value: firestore.Increment(-1)},
	})
	return err
}

func (c client) UpdateStatus(ctx context.Context, id string, status SimulationStatus) error {
	_, err := c.client.Collection(SimulationsCollection).Doc(id).Set(ctx, map[string]interface{}{
		"status":            status,
		"status_updated_at": firestore.ServerTimestamp,
	}, firestore.MergeAll)
	return err
}

func (c client) Close() error {
	return c.client.Close()
}
