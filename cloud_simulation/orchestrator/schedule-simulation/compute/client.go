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
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/protobuf/proto"
)

type InstanceConfig struct {
	InstanceID        string
	InstanceType      string
	IP                string
	VMImage           string
	DiskSize          int64
	Subnet            string
	MaxRunDuration    int64
	ServiceAccount    string
	DockerImage       string
	DocumentID        string
	SimulationID      string
	SimulationBucket  string
	MaxSimulationTime int
	MaxReportCount    int
	FinishFunctionURL string
	RegistryRegion    string
}

type Client interface {
	CreateInstance(ctx context.Context, projectID string, zone string, databaseId string, config InstanceConfig) (string, string, error)
	Close() error
}

type client struct {
	instancesClient *compute.InstancesClient
	logger          *slog.Logger
}

func NewClient(ctx context.Context, logger *slog.Logger) (Client, error) {
	computeClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, err
	}
	return &client{instancesClient: computeClient, logger: logger}, nil
}

func (cc client) Close() error {
	err := cc.instancesClient.Close()
	return err
}

// Utility function to sanitize SimulationID to comply with GCP instance name restrictions
func sanitizeInstanceName(simulationID string) string {
	// Define a regex to match valid instance names based on GCP rules.
	validNameRegex := regexp.MustCompile(`(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?)`)

	// Truncate to a max of 63 characters ("sim-" and the id)
	maxLength := 59
	if len(simulationID) > maxLength {
		simulationID = simulationID[:maxLength]
	}

	// Convert to lowercase and replace invalid characters with "-"
	simulationID = strings.ToLower(simulationID)
	simulationID = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, simulationID)

	// Ensure it ends with a letter or number
	if simulationID[len(simulationID)-1] == '-' {
		simulationID = simulationID[:len(simulationID)-1] + "a"
	}

	// Add sim- prefix for better identification
	simulationID = "sim-" + simulationID

	// Ensure it matches the valid regex, just as a final safety check
	if !validNameRegex.MatchString(simulationID) {
		panic(fmt.Sprintf("Sanitized instance name does not comply with GCP regex: %s", simulationID))
	}

	return simulationID
}

// Adjusted CreateInstance method
func (cc client) CreateInstance(ctx context.Context, projectID string, zone string, databaseID string, config InstanceConfig) (string, string, error) {
	// Sanitize the instance name
	instanceName := sanitizeInstanceName(config.SimulationID)

	cc.logger.Info("Creating instance for sanitized id", "sanitized_id", instanceName, "simulation_id", config.SimulationID)

	req := &computepb.InsertInstanceRequest{
		Project: projectID,
		Zone:    zone,
		InstanceResource: &computepb.Instance{
			Name:                    proto.String(instanceName),
			MachineType:             proto.String(fmt.Sprintf("projects/%s/zones/%s/machineTypes/%s", projectID, zone, config.InstanceType)),
			MinCpuPlatform:          proto.String("Intel Haswell"),
			AdvancedMachineFeatures: buildAdvancedMachineFeatures(),
			Disks:                   buildDisks(projectID, zone, instanceName, config),
			NetworkInterfaces:       buildNetworkInterfaces(config),
			Scheduling:              buildScheduling(config),
			ServiceAccounts:         buildServiceAccounts(config),
			ShieldedInstanceConfig:  buildShieldedInstanceConfig(),
			Labels:                  buildLabels(),
			Metadata:                buildMetadata(projectID, zone, databaseID, instanceName, config),
		},
	}

	op, err := cc.instancesClient.Insert(ctx, req)
	if err != nil {
		return "", "", fmt.Errorf("unable to create instance: %w", err)
	}

	if err = op.Wait(ctx); err != nil {
		return "", "", fmt.Errorf("unable to wait for the operation: %w", err)
	}

	cc.logger.Info("Instance created successfully", "simulation_id", config.SimulationID)

	getInstance := &computepb.GetInstanceRequest{
		Instance: instanceName,
		Project:  projectID,
		Zone:     zone,
	}

	instance, err := cc.instancesClient.Get(ctx, getInstance)
	if err != nil {
		return "", "", fmt.Errorf("unable to get instance details: %w", err)
	}

	if len(instance.NetworkInterfaces) > 0 && instance.NetworkInterfaces[0].NetworkIP != nil {
		privateIP := *instance.NetworkInterfaces[0].NetworkIP
		return instanceName, privateIP, nil
	}

	return "", "", fmt.Errorf("unable to find private IP for the instance")
}
