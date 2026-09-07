# Copyright 2026 Google LLC
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

"""Build rules for generating Go mock libraries using gomock."""

load("@rules_go//docs/go/extras:extras.bzl", "gomock")
load("@rules_go//go:def.bzl", "go_library")

def go_mock_library(
        name,
        src,
        importpath,
        deps = [],
        visibility = ["//visibility:public"]):
    """Generates a mock implementation of a go source file using mockgen, and builds it as a test-only go_library.

    Args:
        name: The name of the mock library.
        src: The source file to generate a mock for.
        importpath: The import path for the generated mock library.
        deps: Additional dependencies for the mock library.
        visibility: The visibility of the mock library.
    """
    out = "mock/" + src

    if not importpath.endswith("/mock"):
        fail("Invalid importpath '%s'. It must end with /mock" % importpath)
    source_importpath = importpath.removesuffix("/mock")

    gomock(
        name = name + "_gen",
        out = out,
        mockgen_tool = "@org_uber_go_mock//mockgen:mockgen",
        source = src,
        source_importpath = source_importpath,
    )

    go_library(
        name = name,
        testonly = True,
        srcs = [out],
        importpath = importpath,
        visibility = visibility,
        deps = deps + ["@org_uber_go_mock//gomock"],
    )
