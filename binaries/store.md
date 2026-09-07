---
title: tariboy-store
description: A standalone image registry server that stores agent-image blobs and serves a token-authenticated push/pull API.
sidebar:
  label: Image registry
  icon: server
---

A standalone **image registry** server (`cmd/tariboy-store`,
`internal/storesvc` + `internal/storeui`): stores agent-image blobs plus a SQLite
catalog/token DB and serves an HTTP(S) push/pull API with bearer-token auth.
Uploads are capped at 256 MiB and are digest-checked and manifest-validated
before replacing a published image. Slow clients are bounded by HTTP header and
idle timeouts.

## Flags

- `--addr` — default `:8443`,
- `--data-dir` — required,
- `--db`,
- `--tls-cert` / `--tls-key`,
- `--token-file`,
- `--anon-pull`,
- `--allow-insecure`.

Operators interact with it via `tariboy push` / `pull` / `login`. It also
serves its own store UI (`internal/storeui`), distinct from the
[daemon's embedded web UI](/docs/architecture/web-ui).
