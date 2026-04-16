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

"""
Macro for wrapping a tool so that it runs in the current working directory when
run via `bazel run`, instead of somewhere in Bazel's output directory.
"""

load("@rules_shell//shell:sh_binary.bzl", "sh_binary")

def tool_binary(name, tool):
    sh_binary(
        name = name,
        srcs = ["bazel-run.sh"],
        args = ["$(execpath {})".format(tool)],
        deps = [
            "@bazel_tools//tools/bash/runfiles",
        ],
        data = [tool],
    )
