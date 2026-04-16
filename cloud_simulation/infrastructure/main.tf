/*
 *  Copyright 2025 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

resource "google_project_service" "enable_services" {
  for_each = toset([
    "firestore.googleapis.com",
    "cloudfunctions.googleapis.com",
    "eventarc.googleapis.com",
    "cloudscheduler.googleapis.com",
  ])
  service = each.key
}

module "cloud_storage" {
  source  = "terraform-google-modules/cloud-storage/google"
  version = "9.0.1"

  project_id = var.project_id
  names      = ["simulation_files${var.environment == "" ? "" : "-${var.environment}"}"]
  prefix     = var.project_id
  location   = var.location
}

module "firestore" {
  source = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/firestore?ref=v37.1.0&depth=1"

  project_id = var.project_id
  database = {
    name        = "${var.project_id}-simulator${var.environment == "" ? "" : "-${var.environment}"}"
    type        = "FIRESTORE_NATIVE"
    location_id = var.location_id
  }
  indexes = local.firestore_indexes
}

# Initialize counter for running simulations
resource "google_firestore_document" "vm_instance_counter" {
  project     = var.project_id
  database    = module.firestore.firestore_database.name
  collection  = "running-vms"
  document_id = "counter"
  fields = jsonencode({
    counter = {
      integerValue = "0"
    }
  })

  lifecycle {
    ignore_changes = [fields]
  }
}

module "simulation_queue" {
  source     = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/pubsub?ref=v37.1.0&depth=1"
  project_id = var.project_id
  name       = "simulation-queue${var.environment == "" ? "" : "-${var.environment}"}"
  schema = {
    msg_encoding = "JSON"
    schema_type  = "AVRO"
    definition = jsonencode({
      "type" = "record",
      "name" = "Avro",
      "fields" = [
        {
          "name" = "id",
          "type" = "string"
        },
        {
          "name" = "owner",
          "type" = "string"
        },
        {
          "name" = "file_path",
          "type" = "string"
        },
        {
          "name" = "build_id",
          "type" = "string"
        },
        {
          "name" = "instance_type",
          "type" = "string"
        },
        {
          "name" = "max_simulation_time",
          "type" = "int",
        },
        {
          "name" = "max_report_count",
          "type" = "int",
        }
      ]
    })
  }

  subscriptions = {
    "simulation-requests${var.environment == "" ? "" : "-${var.environment}"}" = {
      ack_deadline_seconds         = 600
      enable_exactly_once_delivery = true
      enable_message_ordering      = true
      retain_acked_messages        = false
    }
  }
}

module "cloud_storage_function_code" {
  source  = "terraform-google-modules/cloud-storage/google"
  version = "9.0.1"

  project_id = var.project_id
  names      = ["simulation_functions_code${var.environment == "" ? "" : "-${var.environment}"}"]
  prefix     = var.project_id
  location   = var.location
}



locals {
  database_id   = split("/", trimsuffix(module.firestore.firestore_database.id, "/"))
  database_name = tostring(element(local.database_id, length(local.database_id) - 1))
}

resource "google_vpc_access_connector" "connector" {
  count          = try(var.vpc_connector.create, false) == true ? 1 : 0
  name           = "${var.vpc_connector.name}${var.environment == "" ? "" : "-${var.environment}"}"
  ip_cidr_range  = var.vpc_connector_config.ip_cidr_range
  network        = var.vpc_connector_config.network
  max_instances  = try(var.vpc_connector_config.instances.max, null)
  min_instances  = try(var.vpc_connector_config.instances.min, null)
  max_throughput = try(var.vpc_connector_config.throughput.max, null)
  min_throughput = try(var.vpc_connector_config.throughput.min, null)
}

module "simulation_orchestrator_function" {
  source           = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/cloud-function-v2?ref=v37.1.0&depth=1"
  project_id       = var.project_id
  region           = var.default_region
  name             = "simulation-orchestrator-receive-requests${var.environment == "" ? "" : "-${var.environment}"}"
  bucket_name      = module.cloud_storage_function_code.name
  service_account  = var.receive_request_function_sa != null ? var.receive_request_function_sa : module.receive_request_function_sa[0].email
  ingress_settings = var.ingress_settings
  vpc_connector = (
    var.vpc_connector == null
    ? null
    : {
      create          = false
      name            = try(var.vpc_connector.create, false) == false ? var.vpc_connector.name : google_vpc_access_connector.connector[0].id
      egress_settings = var.vpc_connector.egress_settings
    }
  )
  bundle_config = {
    path = "../orchestrator/receive-request/"
    folder_options = {
      excludes     = ["*_test.go"]
    }
  }

  function_config = {
    entry_point = "HandleSimulationRequest"
    runtime     = "go125"
  }

  environment_variables = {
    GCP_PROJECT            = var.project_id
    FIRESTORE_DATABASE     = local.database_name
    PUBSUB_TOPIC_ID        = element(split("/", module.simulation_queue.topic.id), length(split("/", module.simulation_queue.topic.id)) - 1)
    SIMULATION_BUCKET      = var.input_files_bucket != null ? var.input_files_bucket : "gs://${module.cloud_storage.name}"
    AGENT_DOCKER_IMAGE     = var.agent_docker_image
    SUPPORTED_AGENT_BUILDS = jsonencode(var.supported_agent_builds)
  }

  depends_on = [module.firestore]
}

module "simulation_deletion_function" {
  source           = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/cloud-function-v2?ref=v37.1.0&depth=1"
  project_id       = var.project_id
  region           = var.default_region
  name             = "simulation-orchestrator-delete-simulation${var.environment == "" ? "" : "-${var.environment}"}"
  bucket_name      = module.cloud_storage_function_code.name
  service_account  = var.delete_simulation_function_sa != null ? var.delete_simulation_function_sa : module.delete_simulation_function_sa[0].email
  ingress_settings = var.ingress_settings
  vpc_connector = (
    var.vpc_connector == null
    ? null
    : {
      create          = false
      name            = try(var.vpc_connector.create, false) == false ? var.vpc_connector.name : google_vpc_access_connector.connector[0].id
      egress_settings = var.vpc_connector.egress_settings
    }
  )
  bundle_config = {
    path = "../orchestrator/delete-simulation/"
    folder_options = {
      excludes     = ["*_test.go"]
    }
  }

  function_config = {
    entry_point = "HandleCancellationRequest"
    runtime     = "go125"
  }

  environment_variables = {
    GCP_PROJECT        = var.project_id
    FIRESTORE_DATABASE = local.database_name
    COMPUTE_ZONE       = var.default_zone
  }

  depends_on = [module.firestore]
}

module "simulation_finisher_function" {
  source           = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/cloud-function-v2?ref=v37.1.0&depth=1"
  project_id       = var.project_id
  region           = var.default_region
  name             = "simulation-orchestrator-finish-simulation${var.environment == "" ? "" : "-${var.environment}"}"
  bucket_name      = module.cloud_storage_function_code.name
  service_account  = var.simulation_finisher_sa != null ? var.simulation_finisher_sa : module.simulation_finisher_sa[0].email
  ingress_settings = var.ingress_settings
  vpc_connector = (
    var.vpc_connector == null
    ? null
    : {
      create          = false
      name            = try(var.vpc_connector.create, false) == false ? var.vpc_connector.name : google_vpc_access_connector.connector[0].id
      egress_settings = var.vpc_connector.egress_settings
    }
  )
  bundle_config = {
    path = "../orchestrator/finish-simulation/"
    folder_options = {
      excludes     = ["*_test.go"]
    }
  }

  function_config = {
    entry_point = "FinishSimulation"
    runtime     = "go125"
  }

  environment_variables = {
    GCP_PROJECT        = var.project_id
    FIRESTORE_DATABASE = local.database_name
    COMPUTE_ZONE       = var.default_zone
  }

  depends_on = [module.firestore]
}

module "simulation_reader_function" {
  source           = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/cloud-function-v2?ref=v37.1.0&depth=1"
  project_id       = var.project_id
  region           = var.default_region
  name             = "simulation-reader${var.environment == "" ? "" : "-${var.environment}"}"
  bucket_name      = module.cloud_storage_function_code.name
  service_account  = var.simulation_reader_function_sa != null ? var.simulation_reader_function_sa : module.simulation_reader_function_sa[0].email
  ingress_settings = var.ingress_settings
  vpc_connector = (
    var.vpc_connector == null
    ? null
    : {
      create          = false
      name            = try(var.vpc_connector.create, false) == false ? var.vpc_connector.name : google_vpc_access_connector.connector[0].id
      egress_settings = var.vpc_connector.egress_settings
    }
  )
  bundle_config = {
    path = "../orchestrator/read-simulation/"
    folder_options = {
      excludes     = ["*_test.go"]
    }
  }

  function_config = {
    entry_point = "HandleReadRequest"
    runtime     = "go125"
  }

  environment_variables = {
    GCP_PROJECT        = var.project_id
    FIRESTORE_DATABASE = local.database_name
    MAX_VM_COUNT       = var.max_vm_count
  }
}

data "google_compute_subnetwork" "simulation_instances" {
  name   = var.subnet_name
  region = var.default_region
}

locals {
  subscription_id   = split("/", trimsuffix(module.simulation_queue.subscription_id["simulation-requests${var.environment == "" ? "" : "-${var.environment}"}"], "/"))
  subscription_name = tostring(element(local.subscription_id, length(local.subscription_id) - 1))
}

resource "google_eventarc_trigger" "pubsub_trigger" {
  name            = "scheduler-pubsub-trigger${var.environment == "" ? "" : "-${var.environment}"}"
  location        = "us-central1" // serverless region
  project         = var.project_id
  service_account = var.scheduler_trigger_sa != null ? var.scheduler_trigger_sa : module.scheduler_trigger_sa[0].email
  matching_criteria {
    attribute = "type"
    value     = "google.cloud.pubsub.topic.v1.messagePublished"
  }

  transport {
    pubsub {
      topic = module.simulation_queue.topic.id
    }
  }


  destination {
    cloud_run_service {
      service = module.scheduler_function.function_name
      region  = var.default_region
      path    = "/"
    }
  }
}


resource "google_eventarc_trigger" "firestore_trigger" {
  name                    = "scheduler-firestore-trigger-${var.default_region}${var.environment == "" ? "" : "-${var.environment}"}"
  location                = var.location_id
  project                 = var.project_id
  service_account         = var.scheduler_trigger_sa != null ? var.scheduler_trigger_sa : module.scheduler_trigger_sa[0].email
  event_data_content_type = "application/protobuf"

  matching_criteria {
    attribute = "type"
    value     = "google.cloud.firestore.document.v1.updated"
  }

  matching_criteria {
    attribute = "database"
    value     = module.firestore.firestore_database.name
  }

  matching_criteria {
    attribute = "document"
    value     = "running-vms/counter"
  }

  destination {
    cloud_run_service {
      service = module.scheduler_function.function_name
      region  = var.default_region
      path    = "/"
    }
  }
}

module "scheduler_function" {
  source           = "github.com/GoogleCloudPlatform/cloud-foundation-fabric//modules/cloud-function-v2?ref=v37.1.0&depth=1"
  project_id       = var.project_id
  region           = var.default_region
  name             = "simulation-scheduler${var.environment == "" ? "" : "-${var.environment}"}"
  bucket_name      = module.cloud_storage_function_code.name
  service_account  = var.scheduler_function_sa != null ? var.scheduler_function_sa : module.scheduler_function_sa[0].email
  ingress_settings = "ALLOW_INTERNAL_ONLY" //var.ingress_settings
  vpc_connector = (
    var.vpc_connector == null
    ? null
    : {
      create          = false
      name            = try(var.vpc_connector.create, false) == false ? var.vpc_connector.name : google_vpc_access_connector.connector[0].id
      egress_settings = var.vpc_connector.egress_settings
    }
  )

  bundle_config = {
    path = "../orchestrator/schedule-simulation/"
    folder_options = {
      excludes     = ["*_test.go"]
    }
  }

  function_config = {
    entry_point = "HandleScheduleTrigger"
    runtime     = "go125"
  }

  environment_variables = {
    GCP_PROJECT             = var.project_id
    REGISTRY_REGION         = var.registry_region != null ? var.registry_region : var.default_region
    FIRESTORE_DATABASE      = local.database_name
    COMPUTE_ZONE            = var.default_zone
    PUBSUB_SUBSCRIPTION     = local.subscription_name
    VM_SERVICEACCOUNT       = var.simulation_agent_sa != null ? var.simulation_agent_sa : module.simulation_agent_sa[0].email
    MAX_SIMULATION_DURATION = 4 * 60 * 60 //4 hrs to sec
    SUBNET                  = data.google_compute_subnetwork.simulation_instances.id
    DISK_SIZE               = "70"
    MAX_VM_COUNT            = var.max_vm_count
    VM_IMAGE                = var.vm_image
    SIMULATION_BUCKET       = module.cloud_storage.name
    FINISH_FUNCTION_URL     = module.simulation_finisher_function.uri
  }

  depends_on = [module.firestore]
}

resource "google_cloud_scheduler_job" "simulator_instances_cleanup_scheduler" {
  name      = "simulation-instance-cleanup-scheduler${var.environment == "" ? "" : "-${var.environment}"}"
  schedule  = "0 8 * * *" # Runs every day at 8 AM
  time_zone = "Etc/UTC"

  http_target {
    http_method = "POST"
    uri         = module.simulation_deletion_function.uri

    oidc_token {
      service_account_email = var.cleanup_scheduler_sa != null ? var.cleanup_scheduler_sa : module.cleanup_scheduler_sa[0].email
    }
  }
}
