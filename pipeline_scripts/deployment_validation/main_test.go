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
	if err := run(cfg); err != nil {
		t.Fatalf("E2E Test Failed: %v", err)
	}
}
