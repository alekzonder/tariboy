---
title: image-creator
description: Build a runnable Tariboy image from any source directory readable by the daemon.
sidebar:
  label: image-creator
  icon: package-plus
---

`image-creator` allows an agent to publish a new immutable image to the current
host's image store. It is optional and is not included in the official Store's
`basic` source.

## Capability surface

Create a directory containing `Tariboyfile.yaml`, then build it:

```bash
scripts/image_creator.sh build \
  --name reviewer \
  --tag v1 \
  --path ./reviewer-image
```

`--name` and `--path` are required. The tag defaults to `latest`. A successful
response identifies the name, tag, digest, and layer count. The new immutable
image is stored on the same host and can be assigned to an agent through the
normal image workflow.

## Source paths

The daemon resolves a relative `--path` against the managed
`agents/<agent>/workdir`, independently of the configured CWD. An absolute path
may select any directory readable by the daemon process. A relative path may
also contain `..` or pass through a symlink to select such a directory.

Skill and prompt paths use the ordinary image builder rules, including explicit
absolute paths and source-relative declared skills. Grant `image-creator` only
to agents trusted to read and package image sources available to the daemon
account.

## Manifest and plugin validation

New sources must use schema v2 and preserve an explicit plugin list plus ordered
prompt/runtime template. Schema-v1 source builds fail with migration guidance.
Built-in names are validated against Tariboy's registry; external names must
resolve to installed plugin metadata on the same daemon.

The build writes to the daemon's shared immutable image store. It does not
assign the image to the creating agent or change a running iteration.

## Prompt integration

```yaml Tariboyfile.yaml
plugins:
  - name: image-creator
skills:
  - dir: ../../skills/image-creator
```

The packaged image skill teaches the authoring command. The capability is the
authorization to build from directories readable by the daemon account.

## Failure behavior

Invalid refs, missing or unreadable manifests, unknown plugins, duplicate
schema-v2 plugins, unresolved prompt files, and normal image validation errors
fail the build without publishing the requested ref.

## Related reference

- [Images](/docs/images)
- [Image and group lifecycle](/docs/images-and-groups)
- [Operator command reference](/docs/reference/commands#operator-commands)
