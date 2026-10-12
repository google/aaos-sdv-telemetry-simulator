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
	"testing"
)

func TestDefineFlagsOnSet_Defaults(t *testing.T) {
	var cfg config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	defineFlagsOnSet(fs, &cfg)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("fs.Parse(nil) failed: %v", err)
	}
	if cfg.region != defaultRegion {
		t.Errorf("cfg.region = %q, want %q", cfg.region, defaultRegion)
	}
	if cfg.environment != defaultEnvironment {
		t.Errorf("cfg.environment = %q, want %q", cfg.environment, defaultEnvironment)
	}
	if cfg.buildID != defaultBuildID {
		t.Errorf("cfg.buildID = %q, want %q", cfg.buildID, defaultBuildID)
	}
	if cfg.instanceType != defaultInstanceType {
		t.Errorf("cfg.instanceType = %q, want %q", cfg.instanceType, defaultInstanceType)
	}
	if cfg.testFilePath != defaultTestFilePath {
		t.Errorf("cfg.testFilePath = %q, want %q", cfg.testFilePath, defaultTestFilePath)
	}
	if cfg.localTestFilesDir != defaultLocalTestDir {
		t.Errorf("cfg.localTestFilesDir = %q, want %q", cfg.localTestFilesDir, defaultLocalTestDir)
	}
	if cfg.timeout != defaultTimeout {
		t.Errorf("cfg.timeout = %v, want %v", cfg.timeout, defaultTimeout)
	}
	if err := validateConfig(cfg); err == nil {
		t.Errorf("validateConfig(default cfg) = nil, want error for missing --project-id and --owner")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config
		wantErr bool
	}{
		{
			name:    "missing projectID and owner returns error",
			cfg:     config{},
			wantErr: true,
		},
		{
			name: "missing owner returns error",
			cfg: config{
				projectID: "test-project",
			},
			wantErr: true,
		},
		{
			name: "missing projectID returns error",
			cfg: config{
				owner: "tester@example.com",
			},
			wantErr: true,
		},
		{
			name: "both projectID and owner present succeeds",
			cfg: config{
				projectID: "test-project",
				owner:     "tester@example.com",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig(%+v) error = %v, wantErr %v", tt.cfg, err, tt.wantErr)
			}
		})
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
