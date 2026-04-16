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
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"cloud.google.com/go/pubsub"
)

var (
	receivingError    error
	SimulationRequest AvroRecord
	ErrNoMessages     = errors.New("no messages available")
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
	PullMessage(ctx context.Context, subscription_id string) (*AvroRecord, error)
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

func (ps client) PullMessage(ctx context.Context, subscription_id string) (*AvroRecord, error) {
	subscription := ps.client.Subscription(subscription_id)
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	messageReceived := false
	var receivingError error
	var SimulationRequest AvroRecord

	err := subscription.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
		if messageReceived {
			return
		}

		defer cancel() // Cancel the context after processing one message
		messageReceived = true

		ps.logger.Info("Message data", "data", string(m.Data))
		err := json.Unmarshal(m.Data, &SimulationRequest)
		if err != nil {
			ps.logger.Error("Error parsing message", "error", err)
			receivingError = fmt.Errorf("error parsing message: %w", err)
			return
		}

		v := reflect.ValueOf(SimulationRequest)
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).IsZero() {
				fieldName := v.Type().Field(i).Name
				receivingError = fmt.Errorf("missing required fields: %v", fieldName)
				return
			}
		}

		m.Ack() // Acknowledge that we've consumed the message.
	})

	if err != nil && err != context.Canceled {
		return &AvroRecord{}, err
	}

	if !messageReceived {
		return nil, nil
	}

	if receivingError != nil {
		return &AvroRecord{}, receivingError
	}

	ps.logger.Info("Pub/Sub message pulled, parsed, and acknowledged.", "simulation_id", SimulationRequest.ID)
	return &SimulationRequest, nil
}

func (ps client) Close() error {
	err := ps.client.Close()
	return err
}
