---
name: image-creator
description: Use when creating or revising a Tariboy agent image from a Tariboyfile, prompt layers, plugins, and packaged skills.
---

# Agent Image Authoring

Create a schema-v2 `Tariboyfile.yaml` with explicit ordered plugins, packaged
skills, prompt files, and runtime placeholders. Build with:

`tools image build --name <name> [--tag <tag>] --path <source-dir>`

For reproducible production images, vendor shared prompt dependencies, record
them in `tariboy.lock.yaml`, use an immutable tag, and avoid absolute,
`$STORE`, or `$CURRENT_VERSION_STORE` inputs. Local source paths use `./`.
