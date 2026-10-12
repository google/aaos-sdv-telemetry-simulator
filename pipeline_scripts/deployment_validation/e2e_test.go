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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

var e2eCfg config

func init() {
	defineFlags(&e2eCfg)
}

func TestMain(m *testing.M) {
	flag.Parse()

	if e2eCfg.localTestFilesDir != "" {
		r, err := runfiles.New()
		if err != nil {
			log.Fatalf("failed to create runfiles object: %v", err)
		}
		testDataPath, err := r.Rlocation(e2eCfg.localTestFilesDir)
		if err != nil {
			log.Fatalf("failed to find runfiles path for %q: %v", e2eCfg.localTestFilesDir, err)
		}
		if _, err := os.Stat(testDataPath); err != nil {
			log.Fatalf("local test files directory %q (resolved to %q) does not exist: %v",
				e2eCfg.localTestFilesDir, testDataPath, err)
		}
		fmt.Printf("✅ Resolved 'testdata' path to: %s\n", testDataPath)
		e2eCfg.localTestFilesDir = testDataPath
	}

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestE2E(t *testing.T) {
	if err := run(e2eCfg); err != nil {
		t.Fatalf("E2E Test Failed: %v", err)
	}
}
