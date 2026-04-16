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

variable "project_id" {
  type = string
}

variable "environment" {
  type        = string
  description = "Add envrionment name to use as postfix for resources. (Optional)"
  default     = ""

}

variable "default_region" {
  description = "Default Region for resources if no other provided."
  type        = string
}
variable "default_zone" {
  description = "Default Zone for resources if no other provided."
  type        = string
}
variable "location" {
  type = string
}
variable "location_id" {
  type = string
}

variable "max_vm_count" {
  description = "Maximum amount of simulations running in parallel. All other requested simulations will be queued. Default: 5"
  type        = number
  default     = 5
}

variable "subnet_name" {
  type = string
}

variable "ingress_settings" {
  description = "Control traffic that reaches the cloud function. Allowed values are ALLOW_ALL, ALLOW_INTERNAL_AND_GCLB and ALLOW_INTERNAL_ONLY ."
  type        = string
  default     = null
}

variable "vm_image" {
  description = "VM image to be used for the simulation instances. The image must be Container Optimized"
  type        = string
  default     = "projects/cos-cloud/global/images/cos-stable-117-18613-164-38" # latest public COS image
}

variable "vpc_connector" {
  description = "VPC connector configuration. Set create to 'true' if a new connector needs to be created."
  type = object({
    create          = bool
    name            = string
    egress_settings = string
  })
  default = null
}

variable "vpc_connector_config" {
  description = "VPC connector network configuration. Must be provided if new VPC connector is being created."
  type = object({
    ip_cidr_range = string
    network       = string
    instances = optional(object({
      max = optional(number)
      min = optional(number, 2)
    }))
    throughput = optional(object({
      max = optional(number, 300)
      min = optional(number, 200)
    }))
  })
  default = null
  validation {
    condition = (
      var.vpc_connector_config == null ||
      try(var.vpc_connector_config.instances, null) != null ||
      try(var.vpc_connector_config.throughput, null) != null
    )
    error_message = "VPC connector must specify either instances or throughput."
  }
}

variable "input_files_bucket" {
  description = "Bucket path with gs:// prefix where the input files are stored."
  type        = string
  default     = null
}

variable "registry_region" {
  description = "The region of the Artifact Registry to configure the credential helper on the VM to pull the agent docker image. Default: var.default_region"
  type        = string
  default     = null
}

variable "agent_docker_image" {
  description = "Docker image with the SDV and agent, without version tag."
  type        = string
}

variable "receive_request_function_sa" {
  description = "Existing Service Account to be used in this deployment. (Optional)"
  type        = string
  default     = null
}
variable "delete_simulation_function_sa" {
  description = "Existing Service Account to be used in this deployment. (Optional)"
  type        = string
  default     = null
}
variable "scheduler_trigger_sa" {
  description = "Existing Service Account to be used in this deployment. (Optional)"
  type        = string
  default     = null
}
variable "scheduler_function_sa" {
  description = "Existing Service Account to be used in this deployment. (Optional)"
  type        = string
  default     = null
}
variable "simulation_agent_sa" {
  description = "Existing Service Account to be used in this deployment. (Optional)"
  type        = string
  default     = null
}
variable "simulation_finisher_sa" {
  description = "Existing Service Account to be used in this deployment. (Optional)"
  type        = string
  default     = null
}
variable "cleanup_scheduler_sa" {
  description = "Existing Service Account to be used in this deployment. (Optional)"
  type        = string
  default     = null
}

variable "simulation_reader_function_sa" {
  description = "Existing Service Account to be used in this deployment. (Optional)"
  type        = string
  default     = null
}

variable "supported_agent_builds" {
  description = "A map of supported agent build tags to their corresponding image fingerprints/digests. When not empty, the receive-request function will validate incoming requests against this list."
  type        = map(string)
  default     = {}
}
