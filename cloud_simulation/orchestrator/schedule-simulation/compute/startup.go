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

package compute

import "fmt"

// generateStartupScript creates a shell script to be run on instance startup.
// This script is responsible for pulling and running the simulation agent Docker container.
// It replaces the deprecated `gce-container-declaration` metadata key.
func generateStartupScript(instanceName, dockerImage string, registryRegion string) string {
	return fmt.Sprintf(`#!/bin/bash
set -ex

echo "Startup script begun. Configuring Docker..."

# We set HOME to a writable location (/home/root) so the credential helper
# can write the .docker/config.json file.
export HOME=/home/root

# Configure docker to authenticate with the specific Artifact Registry region
docker-credential-gcr configure-docker --registries=%s-docker.pkg.dev

echo "Authentication configured. Pulling and starting container..."

# Run the container
docker run --name %s \
    --privileged \
    --volume /dev/kvm:/dev/kvm \
    --restart=always \
    --detach \
    %s

echo "Container started successfully."
`, registryRegion, instanceName, dockerImage)
}
