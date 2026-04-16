#!/bin/bash
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

set -e

ACTION=$1

terraform_init() {
  echo "Initializing Terraform for '${_ENVIRONMENT}' environment..."
  cd cloud_simulation/infrastructure || exit 1
  terraform init -backend-config="environments/${_ENVIRONMENT}/backend.hcl"
}

terraform_apply() {
  echo "Applying Terraform changes..."
  terraform_init
  terraform apply -auto-approve --var-file="environments/${_ENVIRONMENT}/variables.tfvars"
}

terraform_outputs() {
  echo "Fetching Terraform outputs for '${_ENVIRONMENT}' environment..."
  terraform_init
  # Fetch outputs and write them to the workspace for other steps to use
  terraform output -raw vpc_access_connector > /workspace/vpc_access_connector
  terraform output -raw receive_request_function_url > /workspace/receive_request_url
  terraform output -raw delete_function_url > /workspace/delete_url
  terraform output -raw simulation_files_bucket_name > /workspace/simulation_files_bucket_name
  terraform output -raw firestore_database_id > /workspace/database_id
  echo "BUCKET_NAME=$(cat /workspace/simulation_files_bucket_name)"
}


case $ACTION in
  apply)
    terraform_apply
    ;;
  outputs)
    terraform_outputs
    ;;
  *)
    echo "Invalid action: $ACTION"
    echo "Usage: $0 {apply|outputs}"
    exit 1
    ;;
esac
