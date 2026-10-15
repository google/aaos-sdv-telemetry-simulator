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

// Package orchestrator is the root package that Cloud Functions builds for
// every orchestrator function. Terraform uploads this whole directory for
// each function, and the function's entry_point selects one of the handlers
// below.
package orchestrator

import (
	"net/http"

	"simulator.code/orchestrator/delete-simulation"
	"simulator.code/orchestrator/finish-simulation"
	"simulator.code/orchestrator/read-simulation"
	"simulator.code/orchestrator/receive-request"
	"simulator.code/orchestrator/schedule-simulation"
)

// HandleSimulationRequest is the entry point of the receive-request function.
func HandleSimulationRequest(w http.ResponseWriter, r *http.Request) {
	receiver.HandleSimulationRequest(w, r)
}

// HandleScheduleTrigger is the entry point of the simulation-scheduler function.
func HandleScheduleTrigger(w http.ResponseWriter, r *http.Request) {
	scheduler.HandleScheduleTrigger(w, r)
}

// FinishSimulation is the entry point of the finish-simulation function.
func FinishSimulation(w http.ResponseWriter, r *http.Request) {
	finisher.FinishSimulation(w, r)
}

// HandleCancellationRequest is the entry point of the delete-simulation function.
func HandleCancellationRequest(w http.ResponseWriter, r *http.Request) {
	deletion.HandleCancellationRequest(w, r)
}

// HandleReadRequest is the entry point of the read-simulation function.
func HandleReadRequest(w http.ResponseWriter, r *http.Request) {
	reader.HandleReadRequest(w, r)
}
