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

prep_assets() {
  echo "Preparing Web Demo assets..."
  cd cloud_simulation/web_demo || exit 1

  # Substitute environment variables into the app.yaml template
  # Note: Using a different delimiter for sed to avoid conflicts with URLs.
  CORS_ORIGIN="https://${_ENVIRONMENT}-dot-cloud-telemetry-simulation.ey.r.appspot.com"

  sed -i "s|__ENVIRONMENT__|${_ENVIRONMENT}|g; \
          s|__VPC_ACCESS_CONNECTOR__|$(cat /workspace/vpc_access_connector)|g; \
          s|__CORS_ORIGIN__|${CORS_ORIGIN}|g; \
          s|__CLOUD_FUNCTION_RECEIVE_REQUEST__|$(cat /workspace/receive_request_url)|g; \
          s|__CLOUD_FUNCTION_DELETE__|$(cat /workspace/delete_url)|g; \
          s|__OAUTH_CLIENT_ID__|${_WEB_CLIENT_ID}|g; \
          s|__SIMULATION_FILES_BUCKET_NAME__|$(cat /workspace/simulation_files_bucket_name)|g; \
          s|__PROJECT_ID__|${PROJECT_ID}|g" ./backend/app.yaml

  echo "--- Generated ./backend/app.yaml ---"
  cat ./backend/app.yaml
  echo "------------------------"

  sed -i "s|__ENVIRONMENT__|${_ENVIRONMENT}|g; \
          s|__VPC_ACCESS_CONNECTOR__|$(cat /workspace/vpc_access_connector)|g; \
          s|__CLOUD_FUNCTION_RECEIVE_REQUEST__|$(cat /workspace/receive_request_url)|g; \
          s|__CLOUD_FUNCTION_DELETE__|$(cat /workspace/delete_url)|g; \
          s|__OAUTH_CLIENT_ID__|${_WEB_CLIENT_ID}|g; \
          s|__SIMULATION_FILES_BUCKET_NAME__|$(cat /workspace/simulation_files_bucket_name)|g; \
          s|__PROJECT_ID__|${PROJECT_ID}|g" ./frontend/app.yaml

  echo "--- Generated ./frontend/app.yaml ---"
  cat ./frontend/app.yaml
  echo "------------------------"

  # Define the path to the assets directory for clarity
  ASSETS_DIR="./frontend/assets"

  if [ ! -f "$ASSETS_DIR/config.template.json" ]; then
    echo "Error: Template file not found at $ASSETS_DIR/config.template.json"
    exit 1
  fi

  sed "s|__WEB_CLIENT_ID__|${_WEB_CLIENT_ID}|g; \
       s|__DATABASE_ID__|$(cat /workspace/database_id)|g; \
       s|__BACKEND_URL__|${_APP_ENGINE_BACKEND_URL}|g; \
       s|__SIMULATION_FILES_BUCKET_NAME__|$(cat /workspace/simulation_files_bucket_name)|g" \
   "$ASSETS_DIR/config.template.json" > "$ASSETS_DIR/config.json"

  echo "--- Generated ./frontend/assets/config.json ---"
  cat "$ASSETS_DIR/config.json"
  echo "------------------------"
}

deploy() {
  echo "Deploying Web Demo to App Engine..."
  cd cloud_simulation/web_demo || exit 1
  gcloud app deploy ./backend/app.yaml ./frontend/app.yaml --quiet
}

case $ACTION in
  prep_assets) prep_assets ;;
  deploy) deploy ;;
  *) echo "Invalid action: $ACTION. Use 'prep_assets' or 'deploy'." >&2; exit 1 ;;
esac
