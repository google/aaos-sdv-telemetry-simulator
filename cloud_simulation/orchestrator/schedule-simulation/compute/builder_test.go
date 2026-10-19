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

package compute

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	testProjectID  = "test-project"
	testZone       = "europe-west3-a"
	testDatabaseID = "test-project-simulator"
)

// defaultInstanceConfig mirrors the values a default simulation request
// receives today (deployment defaults for image, disk and run duration; the
// validator's default instance type and limits).
func defaultInstanceConfig() InstanceConfig {
	return InstanceConfig{
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

// canonicalJSON renders a proto message as indented JSON.
// BuildInstance produces deterministic output for fixed test inputs, and
// protojson.MarshalOptions ensures byte-stable, formatted output across all test runs.
func canonicalJSON(t *testing.T, m proto.Message) []byte {
	t.Helper()
	opts := protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}
	data, err := opts.Marshal(m)
	if err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
	// protojson multiline formatting inserts non-deterministic spaces after colons
	// via internal/detrand (either ": " or ":  ") to discourage exact string matching.
	// Normalize to ": " to ensure byte-stable golden comparisons across different binaries.
	data = bytes.ReplaceAll(data, []byte(":  "), []byte(": "))
	return append(data, '\n')
}

// compareGolden compares got with the golden file at path. On mismatch every
// differing line is reported, with instructions to run update_golden.
func compareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file %s: %v", path, err)
	}
	if bytes.Equal(got, want) {
		return
	}
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")
	n := len(gotLines)
	if len(wantLines) > n {
		n = len(wantLines)
	}
	for i := 0; i < n; i++ {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			t.Errorf("line %d differs\n  golden: %s\n  got:    %s", i+1, w, g)
		}
	}
	t.Errorf("golden file %s does not match the built instance. If the change is intended, update the golden file by running: bazel run //cloud_simulation/orchestrator/schedule-simulation/compute:update_golden", path)
}

func TestBuildInstanceMatchesGolden(t *testing.T) {
	instance := BuildInstance(testProjectID, testZone, testDatabaseID, defaultInstanceConfig())
	compareGolden(t, filepath.Join("testdata", "golden", "default_instance.json"), canonicalJSON(t, instance))
}

func TestSanitizeInstanceName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "uuid gets the sim- prefix",
			in:   "aa27583c-96a5-4a67-adad-d15e00c0f12a",
			want: "sim-aa27583c-96a5-4a67-adad-d15e00c0f12a",
		},
		{
			name: "upper case is lowered",
			in:   "AA27583C-96A5",
			want: "sim-aa27583c-96a5",
		},
		{
			name: "invalid characters become hyphens",
			in:   "abc_def.ghi jkl",
			want: "sim-abc-def-ghi-jkl",
		},
		{
			name: "trailing hyphen is replaced by a letter",
			in:   "abc-",
			want: "sim-abca",
		},
		{
			name: "only invalid characters still yield a valid name",
			in:   "!!!",
			want: "sim---a",
		},
		{
			name: "long ids are cut to 59 characters before the prefix",
			in:   strings.Repeat("x", 70),
			want: "sim-" + strings.Repeat("x", 59),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeInstanceName(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeInstanceName(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) > 63 {
				t.Errorf("sanitizeInstanceName(%q) is %d characters, GCE allows 63", tc.in, len(got))
			}
		})
	}
}

// An empty simulation id never reaches the scheduler (the receiver generates
// a UUID), but if it did, sanitizeInstanceName panics instead of returning a
// name. Pinned here so a future change to that behaviour is deliberate.
func TestSanitizeInstanceNamePanicsOnEmptyID(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("sanitizeInstanceName(\"\") did not panic")
		}
	}()
	sanitizeInstanceName("")
}
