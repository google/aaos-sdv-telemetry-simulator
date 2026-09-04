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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

var cfg config

func init() {
	defineFlags(&cfg)
}

func TestMain(m *testing.M) {
	flag.Parse()

	if cfg.localTestFilesDir != "" {
		r, err := runfiles.New()
		if err != nil {
			log.Fatalf("failed to create runfiles object: %v", err)
		}
		testDataPath, err := r.Rlocation(cfg.localTestFilesDir)
		if err != nil {
			log.Fatalf("failed to find runfiles path for '%s': %v", cfg.localTestFilesDir, err)
		}
		if _, err := os.Stat(testDataPath); err != nil {
			log.Fatalf("local test files directory '%s' (resolved to '%s') does not exist: %v",
				cfg.localTestFilesDir, testDataPath, err)
		}
		fmt.Printf("✅ Resolved 'testdata' path to: %s\n", testDataPath)
		cfg.localTestFilesDir = testDataPath
	}

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestE2E(t *testing.T) {
	if cfg.projectID == "" && cfg.owner == "" && cfg.serviceURL == "" && cfg.databaseID == "" && cfg.bucketName == "" {
		t.Skip("no E2E flags provided; skipping live deployment validation")
	}
	if err := run(cfg); err != nil {
		t.Fatalf("E2E Test Failed: %v", err)
	}
}

func TestResolveDatabaseID(t *testing.T) {
	tests := []struct {
		name string
		cfg  config
		want string
	}{
		{
			name: "explicit database-id override",
			cfg:  config{projectID: "my-project", environment: "staging", databaseID: "custom-db"},
			want: "custom-db",
		},
		{
			name: "empty environment omits trailing hyphen",
			cfg:  config{projectID: "my-project", environment: ""},
			want: "my-project-simulator",
		},
		{
			name: "non-empty environment appends suffix",
			cfg:  config{projectID: "my-project", environment: "staging"},
			want: "my-project-simulator-staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveDatabaseID(tt.cfg); got != tt.want {
				t.Errorf("resolveDatabaseID(%+v) = %q, want %q", tt.cfg, got, tt.want)
			}
		})
	}
}
func TestResolveBucketName(t *testing.T) {
	tests := []struct {
		name string
		cfg  config
		want string
	}{
		{
			name: "explicit bucket-name override",
			cfg:  config{projectID: "my-project", environment: "staging", bucketName: "custom-bucket"},
			want: "custom-bucket",
		},
		{
			name: "empty environment omits trailing hyphen",
			cfg:  config{projectID: "my-project", environment: ""},
			want: "my-project-simulation_files",
		},
		{
			name: "non-empty environment appends suffix",
			cfg:  config{projectID: "my-project", environment: "staging"},
			want: "my-project-simulation_files-staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveBucketName(tt.cfg); got != tt.want {
				t.Errorf("resolveBucketName(%+v) = %q, want %q", tt.cfg, got, tt.want)
			}
		})
	}
}

func TestResolveReceiveFunctionName(t *testing.T) {
	tests := []struct {
		name string
		cfg  config
		want string
	}{
		{
			name: "empty environment omits trailing hyphen",
			cfg:  config{environment: ""},
			want: "simulation-orchestrator-receive-requests",
		},
		{
			name: "non-empty environment appends suffix",
			cfg:  config{environment: "staging"},
			want: "simulation-orchestrator-receive-requests-staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveReceiveFunctionName(tt.cfg); got != tt.want {
				t.Errorf("resolveReceiveFunctionName(%+v) = %q, want %q", tt.cfg, got, tt.want)
			}
		})
	}
}

func TestResolveTokenAudience(t *testing.T) {
	tests := []struct {
		name        string
		webClientID string
		serviceURL  string
		want        string
	}{
		{
			name:        "uses webClientID when provided",
			webClientID: "custom-client-id.apps.googleusercontent.com",
			serviceURL:  "https://europe-west3-example.cloudfunctions.net/receive",
			want:        "custom-client-id.apps.googleusercontent.com",
		},
		{
			name:        "falls back to serviceURL when webClientID is empty",
			webClientID: "",
			serviceURL:  "https://europe-west3-example.cloudfunctions.net/receive",
			want:        "https://europe-west3-example.cloudfunctions.net/receive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveTokenAudience(tt.webClientID, tt.serviceURL); got != tt.want {
				t.Errorf("resolveTokenAudience(%q, %q) = %q, want %q", tt.webClientID, tt.serviceURL, got, tt.want)
			}
		})
	}
}
