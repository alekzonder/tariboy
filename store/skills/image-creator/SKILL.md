---
name: image-creator
description: Use when creating or revising a Tariboy agent image from a Tariboyfile, prompt layers, plugins, and packaged skills.
---

# Agent Image Authoring

This skill's `scripts/image_creator.sh` launcher lives inside this skill
directory and calls the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

Create a schema-v2 `Tariboyfile.yaml` with explicit plugins, packaged skills,
and prompts. The `prompts` list is the render order:

```yaml
schema_version: 2
plugins:
  - name: context
skills:
  - dir: ./skills/tasks
prompts:
  - file: ./base.md
  - file: ./review.md
```

Invoke this skill's launcher; `--path` names the directory containing the file:

```bash
scripts/image_creator.sh build --name <name> --tag <tag> --path <source-dir>
```

If writing or building is unavailable, return the complete manifest and command
instead of promising to run them.

The built image is stored on this host and can then be selected for an agent.

For reproducible production images, vendor shared prompt dependencies, record
them in `tariboy.lock.yaml`, and avoid absolute, `$STORE`, or
`$CURRENT_VERSION_STORE` inputs. Local source paths use `./`.

Ordinary builds are mutable even when the tag looks versioned; tag spelling
does not make a ref immutable.
If immutable publication is required, use import, registry, or a controlled
release instead; this launcher cannot create it.
