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

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"cloud.google.com/go/pubsub"
)

var (
	receivingError    error
	SimulationRequest AvroRecord
)

type AvroRecord struct {
	ID                string `json:"id"`
	Owner             string `json:"owner"`
	FilePath          string `json:"file_path"`
	BuildID           string `json:"build_id"`
	InstanceType      string `json:"instance_type"`
	MaxSimulationTime int    `json:"max_simulation_time"`
	MaxReportCount    int    `json:"max_report_count"`
}

type Client interface {
	PublishMessage(ctx context.Context, topic string, message AvroRecord) error
	Close() error
}

type client struct {
	client *pubsub.Client
	logger *slog.Logger
}

func NewClient(ctx context.Context, projectID string, logger *slog.Logger) (Client, error) {
	pubsubClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}

	return &client{client: pubsubClient, logger: logger}, nil
}

func (ps client) PublishMessage(ctx context.Context, topic string, message AvroRecord) error {
	t := ps.client.Topic(topic)

	// Convert the AvroRecord to JSON
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error marshaling message to JSON: %w", err)
	}

	result := t.Publish(ctx, &pubsub.Message{
		Data: jsonData,
	})

	// Block until the result is returned and a server-generated
	// ID is returned for the published message.
	id, err := result.Get(ctx)
	if err != nil {
		return fmt.Errorf("error publishing message: %w", err)
	}

	ps.logger.Info("Pub/Sub message published", "id", id)

	return nil
}

func (ps client) Close() error {
	err := ps.client.Close()
	return err
}
