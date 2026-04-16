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

package compute

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

const (
	simulationLabelKey   = "kind"
	simulationLabelValue = "simulation"
)

type Client interface {
	DeleteInstance(ctx context.Context, projectID string, zone string, instanceID string) error
	InstanceExists(ctx context.Context, projectID string, zone string, instanceID string) (bool, error)
	QueryInstances(ctx context.Context, projectID string, zone string, duration int) ([]string, error)
	Close() error
}

type client struct {
	client *compute.InstancesClient
	logger *slog.Logger
}

func NewClient(ctx context.Context, logger *slog.Logger) (Client, error) {
	computeClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, err
	}
	return &client{client: computeClient, logger: logger}, nil
}

func (c client) QueryInstances(ctx context.Context, projectID string, zone string, duration int) ([]string, error) {
	req := &computepb.ListInstancesRequest{
		Project: projectID,
		Zone:    zone,
		Filter:  proto.String(fmt.Sprintf("labels.%s = %q", simulationLabelKey, simulationLabelValue)),
	}

	var instanceIDs []string
	it := c.client.List(ctx, req)
	for {
		instance, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating over instances: %w", err)
		}

		// Calculate the running duration
		creationTime, err := time.Parse(time.RFC3339, *instance.CreationTimestamp)
		if err != nil {
			c.logger.Warn("Failed to parse creation timestamp for instance", "instance", instance.GetName(), "error", err)
			continue
		}
		runningDuration := time.Since(creationTime)

		// Check if the instance has been running for more than the specified duration
		if runningDuration.Hours() > float64(duration) {
			instanceIDs = append(instanceIDs, instance.GetName())
		}
	}

	return instanceIDs, nil
}

func (c client) DeleteInstance(ctx context.Context, projectID string, zone string, instanceID string) error {
	req := &computepb.DeleteInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceID,
	}

	op, err := c.client.Delete(ctx, req)
	if err != nil {
		// If the instance is not found, it means it was already deleted by another process.
		// In this case, we don't need to return an error, as the desired state (instance deleted) is already achieved.
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
			c.logger.Info("Instance not found, assuming it was already deleted.", "instance", instanceID)
			return nil
		}
		return fmt.Errorf("failed to initiate instance deletion for %s: %w", instanceID, err)
	}

	if err := op.Wait(ctx); err != nil {
		// It's possible that the instance was deleted by another process while we were waiting for the operation to complete.
		// In this case, the operation will fail with a "not found" error.
		// We don't need to return an error, as the desired state (instance deleted) is already achieved.
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
			c.logger.Info("Instance not found during deletion wait, assuming it was already deleted.", "instance", instanceID)
			return nil
		}
		return fmt.Errorf("failed to wait for instance deletion of %s: %w", instanceID, err)
	}
	return nil
}

func (c client) Close() error {
	return c.client.Close()
}

func (c client) InstanceExists(ctx context.Context, projectID string, zone string, instanceID string) (bool, error) {
	req := &computepb.GetInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceID,
	}

	_, err := c.client.Get(ctx, req)
	if err == nil {
		// No error means the instance was found.
		return true, nil
	}

	// If the error is a 404 Not Found, it's not a "real" error for our purposes.
	// It simply means the instance doesn't exist.
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && gerr.Code == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("failed to check existence for instance %s: %w", instanceID, err)
}
