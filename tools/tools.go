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

//go:build never

// This package is present to trick the Go toolchain into pulling in tool dependencies.
// See https://github.com/bazel-contrib/rules_go/blob/master/docs/go/core/bzlmod.md#depending-on-tools
// for more information.
package tools

import (
	_ "github.com/bazelbuild/buildtools/buildifier"
	_ "google.golang.org/protobuf"
)

func init() {
	panic("This code should never run.")
}
