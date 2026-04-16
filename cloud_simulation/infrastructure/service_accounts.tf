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

# authoritative roles granted *on* the service accounts to other identities
# non-authoritative roles granted *to* the service accounts on other resources

module "scheduler_trigger_sa" {
  count      = var.scheduler_trigger_sa == null ? 1 : 0
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/iam-service-account?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "scheduler-trigger${var.environment == "" ? "" : "-${var.environment}"}"
  iam_project_roles = {
    "${var.project_id}" = [
      "roles/eventarc.eventReceiver",
      "roles/run.invoker",
    ]
  }
}

module "simulation_agent_sa" {
  count      = var.simulation_agent_sa == null ? 1 : 0
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/iam-service-account?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "simulation-agent${var.environment == "" ? "" : "-${var.environment}"}"
  iam_project_roles = {
    "${var.project_id}" = [
      "roles/run.invoker",
      "roles/storage.objectUser",
    ]
  }
}

module "simulation_finisher_sa" {
  count      = var.simulation_finisher_sa == null ? 1 : 0
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/iam-service-account?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "simulation-finisher-function${var.environment == "" ? "" : "-${var.environment}"}"
  iam_project_roles = {
    "${var.project_id}" = [
      "roles/datastore.user",
      "roles/compute.instanceAdmin.v1"
    ]
  }
}

module "scheduler_function_sa" {
  count      = var.scheduler_function_sa == null ? 1 : 0
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/iam-service-account?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "scheduler-function${var.environment == "" ? "" : "-${var.environment}"}"
  iam_project_roles = {
    "${var.project_id}" = [
      "roles/datastore.user",
      "roles/compute.instanceAdmin.v1",
      "roles/pubsub.subscriber",
      "roles/iam.serviceAccountUser",
    ]
  }
}

module "receive_request_function_sa" {
  count      = var.receive_request_function_sa == null ? 1 : 0
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/iam-service-account?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "receive-request-function${var.environment == "" ? "" : "-${var.environment}"}"
  iam_project_roles = {
    "${var.project_id}" = [
      "roles/datastore.user",
      "roles/storage.objectUser",
    ]
  }
}

module "delete_simulation_function_sa" {
  count      = var.delete_simulation_function_sa == null ? 1 : 0
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/iam-service-account?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "delete-simulation-function${var.environment == "" ? "" : "-${var.environment}"}"
  iam_project_roles = {
    "${var.project_id}" = [
      "roles/datastore.user",
      "roles/compute.instanceAdmin.v1",
      "roles/run.invoker",
    ]
  }
}

module "cleanup_scheduler_sa" {
  count      = var.cleanup_scheduler_sa == null ? 1 : 0
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/iam-service-account?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "cleanup-scheduler${var.environment == "" ? "" : "-${var.environment}"}"
  iam_project_roles = {
    "${var.project_id}" = [
      "roles/iam.serviceAccountUser",
      "roles/run.invoker",
    ]
  }
}

module "simulation_reader_function_sa" {
  count      = var.simulation_reader_function_sa == null ? 1 : 0
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/iam-service-account?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "simulation-reader${var.environment == "" ? "" : "-${var.environment}"}"
  iam_project_roles = {
    "${var.project_id}" = [
      "roles/datastore.user",
    ]
  }
}
