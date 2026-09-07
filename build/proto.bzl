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

"""Build rules for compiling Protocol Buffers."""

load("@protobuf_src//bazel/common:proto_info.bzl", "ProtoInfo")

def _txtpb_to_binpb_impl(ctx):
    protoc = ctx.executable._protoc
    proto_info = ctx.attr.proto_target[ProtoInfo]

    # Gather all descriptor sets from the proto_library and its transitive dependencies
    descriptor_sets = proto_info.transitive_descriptor_sets.to_list()
    descriptor_set_paths = [d.path for d in descriptor_sets]

    proto_files = [src.path for src in proto_info.direct_sources]

    # protoc --encode reads from standard input and writes to standard output
    command = "{protoc} --descriptor_set_in={desc_sets} --encode={msg_name} {proto_files} < {input} > {output}".format(
        protoc = protoc.path,
        desc_sets = ":".join(descriptor_set_paths),
        msg_name = ctx.attr.message_name,
        proto_files = " ".join(proto_files),
        input = ctx.file.src.path,
        output = ctx.outputs.out.path,
    )

    ctx.actions.run_shell(
        inputs = descriptor_sets + [ctx.file.src],
        outputs = [ctx.outputs.out],
        tools = [protoc],
        command = command,
        mnemonic = "ProtocCompile",
    )

    return [DefaultInfo(files = depset([ctx.outputs.out]))]

txtpb_to_binpb = rule(
    implementation = _txtpb_to_binpb_impl,
    attrs = {
        "src": attr.label(allow_single_file = True, mandatory = True, doc = "The .txtpb input file"),
        "out": attr.output(mandatory = True, doc = "The output .binpb binary file"),
        "message_name": attr.string(mandatory = True, doc = "Fully qualified message name (e.g., my.package.MyMessage)"),
        "proto_target": attr.label(providers = [ProtoInfo], mandatory = True, doc = "The proto_library containing the `.proto` file containing the message definition"),
        "_protoc": attr.label(
            default = Label("@protobuf_src//:protoc"),
            executable = True,
            cfg = "exec",
        ),
    },
)
