// Copyright 2026 Google LLC
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

// Package main updates the golden test files for the compute builder.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"simulator.code/orchestrator/schedule-simulation/compute"
)

const (
	testProjectID  = "test-project"
	testZone       = "europe-west3-a"
	testDatabaseID = "test-project-simulator"
)

func defaultInstanceConfig() compute.InstanceConfig {
	return compute.InstanceConfig{
		InstanceType:      "n1-standard-8",
		VMImage:           "projects/cos-cloud/global/images/cos-stable-117-18613-164-38",
		DiskSize:          70,
		Subnet:            "projects/test-project/regions/europe-west3/subnetworks/test-subnet",
		MaxRunDuration:    14400,
		ServiceAccount:    "simulation-agent@example.com",
		DockerImage:       "europe-west3-docker.pkg.dev/test-project/simulation/simulation-agent:latest",
		DocumentID:        "doc-abc-123",
		SimulationID:      "aa27583c-96a5-4a67-adad-d15e00c0f12a",
		SimulationBucket:  "test-project-simulation_files",
		MaxSimulationTime: 300,
		MaxReportCount:    1,
		FinishFunctionURL: "https://finish.example.com/finish",
		RegistryRegion:    "europe-west3",
	}
}

func canonicalJSON(m proto.Message) ([]byte, error) {
	opts := protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}
	data, err := opts.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("protojson.Marshal: %w", err)
	}
	// protojson multiline formatting inserts non-deterministic spaces after colons
	// via internal/detrand (either ": " or ":  ") to discourage exact string matching.
	// Normalize to ": " to ensure byte-stable golden comparisons across different binaries.
	data = bytes.ReplaceAll(data, []byte(":  "), []byte(": "))
	return append(data, '\n'), nil
}

func main() {
	ws := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
	if ws == "" {
		fmt.Fprintln(os.Stderr, "Error: BUILD_WORKSPACE_DIRECTORY is not set. Run via 'bazel run //cloud_simulation/orchestrator/schedule-simulation/compute:update_golden'.")
		os.Exit(1)
	}

	cfg := defaultInstanceConfig()
	inst := compute.BuildInstance(testProjectID, testZone, testDatabaseID, cfg)
	data, err := canonicalJSON(inst)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating canonical JSON: %v\n", err)
		os.Exit(1)
	}

	targetPath := filepath.Join(ws, "cloud_simulation", "orchestrator", "schedule-simulation", "compute", "testdata", "golden", "default_instance.json")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", targetPath, err)
		os.Exit(1)
	}
	fmt.Printf("Successfully updated golden file: %s\n", targetPath)
}
