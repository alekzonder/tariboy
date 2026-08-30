---
name: messages
description: Use when sending requests or replies, subscribing to channels, or recovering queued and dead-lettered Tariboy messages.
---

# Tariboy Messages

The Python script lives inside this skill directory under `scripts/` and calls
the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

Incoming messages are shown in the iteration prompt. Act on each, then close it
with `tools message processed <id> "<result>"`; `tools message reply` replies and
closes it atomically.

- Notify: `tools message send --channel <name> --text <body>`
- Request a reply: `tools request --channel <name> --text <body> [--deadline 5m]`
- Inspect/recover: `tools message ls [--all]`, `tools message dlq`,
  `tools message dlq requeue <id>`
- Subscribe: inspect `tools sources`, then use `tools channel subscribe`,
  `tools channel ls`, or `tools channel unsubscribe`.

Use matchers to narrow existing channel traffic. Use params only when a provider
must produce data for the subscription.
