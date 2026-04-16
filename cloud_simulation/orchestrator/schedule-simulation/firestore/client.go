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

	"cloud.google.com/go/firestore"
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
	CounterField                           = "counter"
)

type CounterDoc struct {
	Counter int `firestore:"counter"`
}

type Client interface {
	SimulationUpdate(ctx context.Context, id string, instance string, ip string) error
	GetSimulationDocID(ctx context.Context, id string) (string, error)
	CheckCounter(ctx context.Context, maxRunningVMs int) (bool, error)
	IncrementCounter(ctx context.Context, maxRunningVMs int) error
	DecrementCounter(ctx context.Context) error
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

func (c client) GetSimulationDocID(ctx context.Context, id string) (string, error) {
	query := c.client.Collection(SimulationsCollection).Where("id", "==", id).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil || len(docs) == 0 {
		return "", err
	}

	return docs[0].Ref.ID, nil
}

func (f client) SimulationUpdate(ctx context.Context, id string, instance string, ip string) error {
	query := f.client.Collection(SimulationsCollection).Where("id", "==", id).Limit(1)
	doc, err := query.Documents(ctx).Next()
	if err != nil {
		return fmt.Errorf("error finding Firestore document for simulation request with id %s: %w", id, err)
	}
	docID := doc.Ref.ID

	updateData := map[string]interface{}{
		"instance_id":       instance,
		"ip":                ip,
		"status":            StatusRunning,
		"started_at":        firestore.ServerTimestamp,
		"status_updated_at": firestore.ServerTimestamp,
	}

	_, err = f.client.Collection(SimulationsCollection).Doc(docID).Set(ctx, updateData, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("error updating Firestore document for simulation request with id %s: %w", id, err)
	}

	f.logger.Info("Successfully updated simulation record", "simulation_id", id)
	return nil
}

func (f client) IncrementCounter(ctx context.Context, maxRunningVMs int) error {
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc := f.client.Collection(RunningVMsCollection).Doc(CounterDocument)
		snapshot, err := tx.Get(doc)
		if err != nil {
			return fmt.Errorf("failed to get counter document: %w", err)
		}

		var counterData CounterDoc
		if err := snapshot.DataTo(&counterData); err != nil {
			return fmt.Errorf("failed to unmarshal counter data: %w", err)
		}

		if counterData.Counter >= maxRunningVMs {
			return status.Errorf(codes.FailedPrecondition, "max VMs reached")
		}
		return tx.Set(doc, map[string]interface{}{
			CounterField: counterData.Counter + 1,
		}, firestore.MergeAll)
	}, firestore.MaxAttempts(5))
	if err != nil {
		if status.Code(err) == codes.Aborted {
			f.logger.Warn("Increment counter Firestore transaction aborted")
			return nil
		}
		return fmt.Errorf("Increment counter Firestore transaction failed: %w", err)
	}
	return nil
}

func (f client) DecrementCounter(ctx context.Context) error {
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc := f.client.Collection(RunningVMsCollection).Doc(CounterDocument)
		snapshot, err := tx.Get(doc)
		if err != nil {
			return fmt.Errorf("failed to get counter document: %w", err)
		}

		var counterData CounterDoc
		if err := snapshot.DataTo(&counterData); err != nil {
			return fmt.Errorf("failed to unmarshal counter data: %w", err)
		}

		return tx.Set(doc, map[string]interface{}{
			CounterField: counterData.Counter - 1,
		}, firestore.MergeAll)
	}, firestore.MaxAttempts(5))
	if err != nil {
		if status.Code(err) == codes.Aborted {
			f.logger.Warn("Decrement counter Firestore transaction aborted")
			return nil
		}
		return fmt.Errorf("Decrement counter Firestore transaction failed: %w", err)
	}

	return nil
}

func (f client) CheckCounter(ctx context.Context, maxRunningVMs int) (bool, error) {
	doc := f.client.Collection(RunningVMsCollection).Doc(CounterDocument)
	snapshot, _ := doc.Get(ctx)

	var counterData CounterDoc
	snapshot.DataTo(&counterData)
	if err := snapshot.DataTo(&counterData); err != nil {
		return false, fmt.Errorf("failed to unmarshal counter data: %w", err)
	}

	if counterData.Counter >= maxRunningVMs {
		return false, nil
	}

	return true, nil
}

func (f client) Close() error {
	err := f.client.Close()
	return err
}
