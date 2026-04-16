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

package finisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/idtoken"
)

// Payload is the data sent to the finish-simulation function.
type Payload struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Zone       string `json:"zone"`
	InstanceID string `json:"instance_id"`
	DocumentID string `json:"document_id"`
	Status     string `json:"status"`
}

// Client defines the interface for communicating with the finish-simulation function.
type Client interface {
	Finish(ctx context.Context, url string, payload Payload) error
}

type client struct {
	httpClient *http.Client
}

// NewClient creates a new finisher client.
func NewClient(ctx context.Context) (Client, error) {
	return &client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Finish sends the final status to the finish-simulation Cloud Function.
// It also adds an Authorization header with an identity token for secure invocation.
func (c *client) Finish(ctx context.Context, url string, payload Payload) error {
	// Create an identity token source for the target Cloud Function URL.
	// The service account of the agent VM needs "Cloud Functions Invoker" role on the function.
	tokenSource, err := idtoken.NewTokenSource(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to create idtoken source: %w", err)
	}

	token, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to get identity token: %w", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal finish payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create finish request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call finish function: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("finish function returned non-success status: %s", resp.Status)
	}

	return nil
}
