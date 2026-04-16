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
	"fmt"
	"strconv"

	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/protobuf/proto"
)

func buildAdvancedMachineFeatures() *computepb.AdvancedMachineFeatures {
	return &computepb.AdvancedMachineFeatures{
		EnableNestedVirtualization: proto.Bool(true),
	}
}

func buildDisks(projectID, zone, instanceName string, config InstanceConfig) []*computepb.AttachedDisk {
	return []*computepb.AttachedDisk{
		{
			AutoDelete: proto.Bool(true),
			Boot:       proto.Bool(true),
			DeviceName: proto.String(instanceName),
			InitializeParams: &computepb.AttachedDiskInitializeParams{
				DiskSizeGb:  proto.Int64(config.DiskSize),
				DiskType:    proto.String(fmt.Sprintf("projects/%s/zones/%s/diskTypes/pd-balanced", projectID, zone)),
				SourceImage: proto.String(config.VMImage),
			},
			Mode: proto.String("READ_WRITE"),
			Type: proto.String("PERSISTENT"),
		},
	}
}

func buildNetworkInterfaces(config InstanceConfig) []*computepb.NetworkInterface {
	return []*computepb.NetworkInterface{
		{
			StackType:  proto.String("IPV4_ONLY"),
			Subnetwork: proto.String(config.Subnet),
		},
	}
}

func buildScheduling(config InstanceConfig) *computepb.Scheduling {
	return &computepb.Scheduling{
		AutomaticRestart:          proto.Bool(false),
		InstanceTerminationAction: proto.String("DELETE"),
		MaxRunDuration: &computepb.Duration{
			Seconds: proto.Int64(config.MaxRunDuration),
		},
		OnHostMaintenance: proto.String("TERMINATE"),
		ProvisioningModel: proto.String("SPOT"),
	}
}

func buildServiceAccounts(config InstanceConfig) []*computepb.ServiceAccount {
	return []*computepb.ServiceAccount{
		{
			Email: proto.String(config.ServiceAccount),
			Scopes: []string{
				"https://www.googleapis.com/auth/cloud-platform",
				"https://www.googleapis.com/auth/logging.write",
				"https://www.googleapis.com/auth/monitoring.write",
				"https://www.googleapis.com/auth/service.management.readonly",
				"https://www.googleapis.com/auth/servicecontrol",
				"https://www.googleapis.com/auth/trace.append",
			},
		},
	}
}

func buildShieldedInstanceConfig() *computepb.ShieldedInstanceConfig {
	return &computepb.ShieldedInstanceConfig{
		EnableIntegrityMonitoring: proto.Bool(true),
		EnableSecureBoot:          proto.Bool(false),
		EnableVtpm:                proto.Bool(true),
	}
}

func buildLabels() map[string]string {
	return map[string]string{
		"kind": "simulation",
	}
}

func buildMetadata(projectID, zone, databaseID, instanceName string, config InstanceConfig) *computepb.Metadata {
	startupScript := generateStartupScript(instanceName, config.DockerImage, config.RegistryRegion)

	return &computepb.Metadata{
		Items: []*computepb.Items{
			{Key: proto.String("project_id"), Value: proto.String(projectID)},
			{Key: proto.String("zone"), Value: proto.String(zone)},
			{Key: proto.String("database_id"), Value: proto.String(databaseID)},
			{Key: proto.String("document_id"), Value: proto.String(config.DocumentID)},
			{Key: proto.String("simulation_id"), Value: proto.String(config.SimulationID)},
			{Key: proto.String("simulation_bucket"), Value: proto.String(config.SimulationBucket)},
			{Key: proto.String("max_simulation_time"), Value: proto.String(strconv.Itoa(config.MaxSimulationTime))},
			{Key: proto.String("max_report_count"), Value: proto.String(strconv.Itoa(config.MaxReportCount))},
			{Key: proto.String("finish_function_url"), Value: proto.String(config.FinishFunctionURL)},
			{Key: proto.String("google-logging-enabled"), Value: proto.String("true")},
			{Key: proto.String("startup-script"), Value: proto.String(startupScript)},
		},
	}
}
