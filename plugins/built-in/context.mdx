---
title: context
description: Read and atomically replace the durable working memory carried between agent iterations.
sidebar:
  label: context
  icon: notebook-text
---

`context` provides one durable working-memory document per agent. It is an
optional capability included in `basic:latest`.

## Capability surface

```bash
scripts/context.sh get
scripts/context.sh set "Current goal, completed work, and next step"
```

`get` returns the complete document. A missing document is equivalent to empty
context. `set` **replaces the entire document**; it is not an append or patch
operation. Include every fact that the next iteration must retain.

## Storage and consistency

The authoritative file is `CONTEXT.md` in the agent's durable root, not in its
configured working directory or unpacked image. Writes use an owner-readable
temporary file in the same directory followed by an atomic rename. Concurrent
reads and writes are serialized, so readers see either the previous complete
document or the next complete document, never a partially written file.

Context survives iterations, image activation, and non-purging reprovisioning.
It is deleted only with the agent's durable data, such as an explicit purging
removal.

## Prompt integration

The capability gates the route, the packaged Store skill explains replacement,
and the runtime marker inserts the current text:

```yaml Tariboyfile.yaml
plugins:
  - name: context
skills:
  - dir: $CURRENT_VERSION_STORE/skills/context
prompts:
  - runtime: context
```

Omitting `runtime: context` leaves the commands available but does not place the
saved document in future iteration prompts.

## Failure behavior and security

An image cannot use `context set` unless it declares the capability. The API
returns `plugin_disabled` before reading or writing the file. The socket fixes
the agent identity, so callers cannot select another agent's context.

Context may contain operational state, but it is not a secret store. Use
Tariboy secrets for credentials, and avoid copying sensitive values into
prompts. Support bundles exclude `CONTEXT.md`.

## Related reference

- [Iteration loop](/docs/architecture/iteration-loop)
- [Security and controls](/docs/security-controls)
- [Images](/docs/images#runtime-placeholders)
