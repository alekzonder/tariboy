# Telegram Plugin Implementation Plan

> Execute only after the plugin operator-surfaces plan passes its phase checks.

**Goal:** Ship an auto-enabled `tariboy-plugin-telegram` that binds one
operator-created Telegram forum supergroup to one daemon, creates a management
topic plus one retained topic per agent, exposes lifecycle/task commands, and
uses existing channel/message delivery for conversation text.

**Architecture:** A standalone supervised Go process owns Bot API polling,
configuration, topic mappings, and Telegram delivery. It serves the existing
plugin Unix HTTP contract, calls the daemon's operator/plugin APIs over its Unix
socket, and uses only `net/http` plus existing Tariboy packages.

**Tech stack:** Go stdlib, Telegram Bot API, existing plugin/channel protocol,
React generic plugin settings renderer, isolated integration tests.

## Task 1: Plugin process and durable state

**Files:**

- Add: `cmd/tariboy-plugin-telegram/main.go`
- Add: `internal/telegramplugin/server.go`
- Add: `internal/telegramplugin/state.go`
- Add: `internal/telegramplugin/state_test.go`

1. Add failing tests for defaults, owner-only token storage, atomic updates,
   normalized signed 64-bit UID lists, offsets, and retained topic mappings.
2. Implement the minimal Unix HTTP server for `/health`, `/routes`, `/action`,
   and `/deliver`, plus the protocol-v1 stdout handshake.
3. Persist config/state in `TARIBOY_PLUGIN_WORKDIR`; never serialize a token in
   status or errors.
4. Run `go test ./internal/telegramplugin -run 'State|Server'`.
5. Commit the process/state slice.

## Task 2: Bot API client and configuration actions

**Files:**

- Add: `internal/telegramplugin/botapi.go`
- Add: `internal/telegramplugin/botapi_test.go`
- Modify: `internal/telegramplugin/server.go`

1. Add an `httptest` Bot API with cases for `getMe`, `getChat`,
   `getChatMember`, `createForumTopic`, `getUpdates`, `sendMessage`, 429
   `retry_after`, 401/403, and redacted errors.
2. Implement one stdlib JSON client with a configurable base URL test seam,
   bounded timeouts, rate-limit delay, and no token-bearing logged URLs.
3. Implement `configure` so `getMe` validates a new token before atomic replace;
   omitted token changes only `allowed_uids`.
4. Treat no token and empty allowlist as healthy disabled states.
5. Run the focused package tests and commit.

## Task 3: Chat setup and topic reconciliation

**Files:**

- Add: `internal/telegramplugin/topics.go`
- Add: `internal/telegramplugin/topics_test.go`
- Modify: `internal/telegramplugin/server.go`

1. Add failing tests for exact supergroup/forum validation, bot topic rights,
   management topic creation, resumable per-agent topic creation, durable
   identity mappings, deleted-agent retention, and missing-thread replacement.
2. Implement `chat_setup` using only Bot API validation and topic creation. Do
   not modify Telegram user rights or forum mode.
3. List agents through the daemon Unix API and reconcile only missing mappings
   at startup and on a bounded ticker.
4. Persist every created topic immediately; preserve the previous chat binding
   until setup completes.
5. Run focused tests and commit.

## Task 4: Ingress authorization and channel publishing

**Files:**

- Add: `internal/telegramplugin/poll.go`
- Add: `internal/telegramplugin/poll_test.go`
- Add: `internal/telegramplugin/daemon.go`
- Modify: `internal/telegramplugin/server.go`

1. Add failing tests for deny-all offset advancement, allowed UID plus exact
   chat/thread checks, anonymous/unknown update rejection, update idempotency,
   ordinary agent-topic text, and stale backlog discard.
2. Implement long polling with persisted offsets and bounded transient backoff.
3. Ensure each mapped agent is subscribed to
   `chat:telegram:<agent-name>` through the normal operator API, then publish
   accepted text through `/api/plugin/publish` using the plugin life token.
4. Advance the offset only after durable acceptance or intentional discard.
5. Run focused tests and commit.

## Task 5: Agent reply delivery

**Files:**

- Add: `internal/telegramplugin/deliver.go`
- Add: `internal/telegramplugin/deliver_test.go`
- Modify: `internal/telegramplugin/server.go`

1. Add failing tests for mapped channel/thread delivery, inbound echo rejection,
   ordered plain-text splitting, acknowledgement only after successful send,
   429 retry, and one missing-thread replacement retry.
2. Decode the existing plugin delivery envelope and send only agent-produced
   replies to the mapped topic.
3. Use Telegram's text limit directly; add no Markdown renderer or dependency.
4. Run focused tests and commit.

## Task 6: Telegram commands

**Files:**

- Add: `internal/telegramplugin/commands.go`
- Add: `internal/telegramplugin/commands_test.go`
- Modify: `internal/telegramplugin/poll.go`

1. Add failing table tests for both topic command sets, `@botname` suffixes,
   implicit agent defaults, remaining-text arguments, safe `/agent set` fields,
   `/help` completeness, unknown commands, and daemon validation errors.
2. Dispatch lifecycle, agent creation/update, and native task operations directly
   to existing daemon operator endpoints. Never publish commands as prompts.
3. Keep one command registry as the source for dispatch and `/help` output.
4. Ignore unauthorized commands silently and reply with errors only to the
   authorized originating topic.
5. Run focused tests and commit.

## Task 7: Bundle, manifest, CLI/UI exposure

**Files:**

- Add: `internal/telegramplugin/manifest.go`
- Modify: `internal/daemon/daemon.go`
- Modify: `Makefile`
- Modify: `desktop/src-tauri/src/bundle.rs`
- Modify: `desktop/src-tauri/src/cli_install.rs`
- Modify: `desktop/src-tauri/src/menu.rs`
- Modify: `desktop/src-tauri/src/remote-install.sh`
- Modify matching packaging tests and smoke scripts that enumerate binaries.

1. Add failing tests for the Telegram manifest contributions and the expanded
   bundled binary set.
2. Declare `telegram configure`, `telegram chat setup`, and `telegram status`,
   plus the generic settings fields/actions. Channels publish and subscribe only
   to `chat:telegram:*`.
3. Build and package `tariboy-plugin-telegram` beside the daemon and ensure it
   before plugin startup. Preserve plugin workdir across upgrades.
4. Update every canonical binary enumeration and checksum/install test; do not
   stage generated Desktop resource directories.
5. Run Go tests, Rust bundle/install tests, and packaging tests; commit.

## Task 8: Documentation and end-to-end verification

**Files:**

- Modify: `README.md`
- Modify: `docs/docs/plugins/index.mdx`
- Modify: `docs/docs/architecture/messaging.mdx`
- Modify: `docs/docs/reference/channels.md`
- Modify: `docs/docs/binaries/operator-cli.mdx`
- Add a focused isolated Telegram plugin integration test under `scripts/` only
  if package-level daemon integration cannot cover the complete channel path.

1. Document prerequisites, setup/status commands, UI workflow, deny-all
   semantics, topic/command behavior, retained topics, security, and recovery.
2. Run the fake-Bot-API isolated daemon integration: Telegram update to bus and
   agent reply back to the same thread. Never use live Tariboy directories or
   `127.0.0.1:9990`.
3. Run `make check`, then `make full-check` because UI/Desktop packaging changed.
4. Run production Desktop Playwright and `tauri-driver` gates required by the
   contributor guide.
5. Run `git diff --check`, inspect the complete diff, and resolve every
   Critical/Important review finding.
