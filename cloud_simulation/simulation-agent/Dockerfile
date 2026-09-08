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

# Build stage: Build the Go executable
FROM golang:1.26 AS agent-builder

COPY agent /agent
WORKDIR /agent
RUN go mod download
RUN go build -o agent-binary .

FROM europe-docker.pkg.dev/android-cuttlefish-artifacts/cuttlefish-orchestration/cuttlefish-orchestration

# Set environment variables
ENV ANDROID_SDK_ROOT=/android/sdk
ENV PATH=$PATH:$ANDROID_SDK_ROOT/emulator:$ANDROID_SDK_ROOT/tools:$ANDROID_SDK_ROOT/tools/bin:$ANDROID_SDK_ROOT/platform-tools

# TODO: Add the host package and image zip to the Docker image
COPY sdv-image-resources/ /opt/android/

RUN apt install -y unzip
RUN tar -xzf /opt/android/cvd-host_package.tar.gz -C /opt/android/
RUN unzip /opt/android/sdv_core_cf-img-*.zip -d /opt/android/

WORKDIR /opt/android/
ENV HOME=/opt/android/

# overwrite ENTRYPOINT from base
ENTRYPOINT ["/usr/bin/env"]

# Add the simulation agent executable
COPY --from=agent-builder /agent/agent-binary /agent/agent-binary

# Run the agent executable directly
CMD ["sh", "-c", "export HOME=/opt/android/ && cd /agent && ./agent-binary"]
