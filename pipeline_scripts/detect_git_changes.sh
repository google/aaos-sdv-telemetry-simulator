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

# Check if we are in a git repo (Automated Triggers) or manual submission (gcloud)
if [ -d ".git" ]; then
  echo "Git repo detected. Running diff logic..."
  git fetch --depth=2 origin "${_BRANCH_NAME}"
  BASE_REF="HEAD~1"
  git diff --name-only "$BASE_REF" HEAD > changed_files.txt
else
  echo "No .git repo detected (likely gcloud builds submit)."
  echo "Simulating changes for development..."
  # Create a dummy list of changed files to force all components to build.
  echo "cloud_simulation/infrastructure/force_test" > changed_files.txt
  echo "cloud_simulation/orchestrator/force_test" >> changed_files.txt
  echo "cloud_simulation/simulation-agent/force_test" >> changed_files.txt
  echo "cloud_simulation/web_demo/force_test" >> changed_files.txt
fi

echo "Files changed:"
cat changed_files.txt

# Sentinel file logic (remains the same)
if grep -q "^cloud_simulation/infrastructure/" changed_files.txt; then
  touch /workspace/tf_changed
fi
if grep -q "^cloud_simulation/orchestrator/" changed_files.txt; then
  touch /workspace/functions_changed
fi
if grep -q "^cloud_simulation/simulation-agent/" changed_files.txt; then
  touch /workspace/agent_changed
fi
if grep -q "^cloud_simulation/web_demo/" changed_files.txt; then
  touch /workspace/web_changed
fi
