---
title: whoami
description: Inspect the authenticated agent identity, working directory, active iteration, and client/daemon versions.
sidebar:
  label: whoami
  icon: fingerprint
---

`whoami` exposes read-only identity information through the per-agent tools
socket. It is a historical core capability and is included in `basic:latest`.
Schema-v2 images must still declare it explicitly.

## Capability surface

```bash
scripts/whoami.sh
```

The daemon response contains:

| Field | Meaning |
| --- | --- |
| `agent` | Authenticated agent name bound to the socket. |
| `cwd` | Effective working directory configured for the agent. |
| `iteration` | Currently running iteration ID, or an empty value when none is running. |
| `daemon_version` | Version of the daemon serving the request. |

The client also knows its own build version. Comparing client and daemon
versions helps diagnose an old agent shim calling a newer daemon. Every agent
API response carries the daemon version header, and the CLI reports version
drift on stderr without changing command output or the exit code.

## Prompt integration

Package the Store skill and place the runtime `identity` marker where the live
identity block belongs:

```yaml Tariboyfile.yaml
plugins:
  - name: whoami
skills:
  - dir: $CURRENT_VERSION_STORE/skills/whoami
prompts:
  - runtime: identity
```

These declarations are independent. Without the skill the procedure is not
available to the harness; without `runtime: identity`, the live identity block
is not inserted automatically.

## State and security

`whoami` creates no durable state and accepts no caller-supplied identity. The
agent name comes from the daemon-owned socket server, not from a command flag or
request body. Run it only inside an agent where `$TARIBOY_TOOLS_SOCKET` is set.

If the capability is absent, the API returns `plugin_disabled`. If the socket
environment is absent, the skill script exits before contacting a daemon.

## Related reference

- [Agent tools](/docs/binaries/agent-tools)
- [Binaries and version drift](/docs/binaries#clientdaemon-version-drift)
- [Iteration loop](/docs/architecture/iteration-loop)
