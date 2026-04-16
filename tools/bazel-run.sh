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

#
# Helper script that executes the provided bazel binary in the
# $BUILD_WORKING_DIRECTORY, i.e., the current working directory of the user when
# they `bazel run`.

# --- begin runfiles.bash initialization v3 ---
# Copy-pasted from the Bazel Bash runfiles library v3.
set -uo pipefail; set +e; f=bazel_tools/tools/bash/runfiles/runfiles.bash
# shellcheck disable=SC1090
source "${RUNFILES_DIR:-/dev/null}/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "${RUNFILES_MANIFEST_FILE:-/dev/null}" | cut -f2- -d' ')" 2>/dev/null || \
  source "$0.runfiles/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.exe.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  { echo>&2 "ERROR: cannot find $f"; exit 1; }; f=; set -e
# --- end runfiles.bash initialization v3 ---

set -euo pipefail

# The runfiles environment variables are needed for `py_binary`- and
# `py_console_script_binary`-based binaries to execute correctly.
runfiles_export_envvars

# The path of the binary is always relative to the Bazel workspace root
# directory, hence, we prepend $BUILD_WORKSPACE_DIRECTORY to it to make it
# absolute. This is necessary for the case where $BUILD_WORKSPACE_DIRECTORY !=
# $BUILD_WORKING_DIRECTORY
BINARY="${BUILD_WORKSPACE_DIRECTORY}/$1"
shift

cd "${BUILD_WORKING_DIRECTORY}"
exec "${BINARY}" "$@"
