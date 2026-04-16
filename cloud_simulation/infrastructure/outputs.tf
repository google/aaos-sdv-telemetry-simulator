# Copyright 2025 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

output "vpc_access_connector" {
  description = "The ID of the VPC Access Connector used."
  value       = google_vpc_access_connector.connector[0].id
}

output "receive_request_function_url" {
  description = "The HTTPS trigger URL for the 'receive requests' Cloud Function."
  value       = module.simulation_orchestrator_function.uri
}

output "delete_function_url" {
  description = "The HTTPS trigger URL for the 'delete simulation' Cloud Function."
  value       = module.simulation_deletion_function.uri
}

output "simulation_files_bucket_name" {
  description = "The name of the Cloud Storage bucket for simulation files."
  value       = module.cloud_storage.name
}

output "firestore_database_id" {
  description = "Database ID of the firestore instance"
  value       = module.firestore.firestore_database.id
}
