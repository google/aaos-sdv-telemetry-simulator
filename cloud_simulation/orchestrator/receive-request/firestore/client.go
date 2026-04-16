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

	"cloud.google.com/go/firestore"
	"simulator.code/receive-request/pubsub"
)

type SimulationStatus string

const (
	StatusRunning         SimulationStatus = "running"
	StatusScheduled       SimulationStatus = "Simulation request received"
	StatusCancelled       SimulationStatus = "cancelled"
	StatusCompleted       SimulationStatus = "completed"
	SimulationsCollection                  = "records"
)

type Client interface {
	Write(ctx context.Context, record pubsub.AvroRecord) error
	Close() error
}

type client struct {
	firestoreClient *firestore.Client
}

func NewClient(ctx context.Context, projectID string, databaseID string) (Client, error) {
	firestoreClient, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, err
	}

	return &client{firestoreClient: firestoreClient}, nil
}

func (f client) Write(ctx context.Context, record pubsub.AvroRecord) error {
	_, _, err := f.firestoreClient.Collection(SimulationsCollection).Add(ctx, map[string]interface{}{
		"id":                  record.ID,
		"owner":               record.Owner,
		"file_path":           record.FilePath,
		"build_id":            record.BuildID,
		"instance_type":       record.InstanceType,
		"max_simulation_time": record.MaxSimulationTime,
		"max_report_count":    record.MaxReportCount,
		"status":              StatusScheduled,
		"received_at":         firestore.ServerTimestamp,
		"status_updated_at":   firestore.ServerTimestamp,
	})
	return err
}

func (f client) Close() error {
	err := f.firestoreClient.Close()
	return err
}
