# Telegram Plugin Integration Design

## Goal

Add a bundled Telegram integration for each `tariboyd` without putting
Telegram-specific behavior in the daemon, CLI, or React application. The work
has two ordered phases:

1. Extend the plugin architecture with declarative operator commands and
   settings UI contributions.
2. Build Telegram as the first bundled plugin using those extensions and the
   existing channel/message delivery path.

The smallest supported deployment is one bot token, one forum supergroup, one
daemon management topic, and one retained topic per agent.

## Telegram platform boundary

A Telegram bot token cannot create a group, convert it to a supergroup, enable
forum mode, or promote users. The operator therefore creates the group and
enables topics in Telegram before binding it to Tariboy.

Tariboy never changes administrator membership or permissions. During setup it
only verifies that:

- the configured chat exists and is a `supergroup`;
- `is_forum` is true;
- the configured bot is a member with the right to manage topics.

If any condition is missing, setup fails with a precise instruction. There is
no MTProto user session and no attempt to work around Bot API limitations.

## Phase 1: declarative plugin operator surfaces

### Manifest contributions

The existing plugin manifest gains two optional contribution groups:

- `operator_commands`: command paths, summaries, typed positional arguments
  and flags, and the plugin action invoked by the command;
- `settings`: sections composed from the existing form primitives needed here
  (`string`, `password`, `integer-list`, status text, and action buttons).

Both groups are data, not executable UI or CLI code. A contribution is scoped
under its plugin name, so the Telegram plugin can contribute `telegram ...`
but cannot replace a core command or another plugin's command. Duplicate paths,
unsupported field types, malformed argument definitions, and unscoped paths
make the manifest invalid.

The current plugin protocol, channel source/sink behavior, generic routes, and
action request/response format remain valid. Plugins without the new fields
behave exactly as before. No second extension protocol is introduced.

### Daemon discovery and invocation

The authenticated operator API exposes the validated contributions of running
plugins. It contains display metadata and schemas only; it never contains
stored secret values.

Command and settings actions use the existing plugin action bridge. The daemon
continues to authenticate and authorize the operator request, selects the
named plugin, validates submitted values against the manifest declaration, and
passes the normalized action payload to the supervised process. Existing
subscription effects remain available to action responses.

Plugin-specific read-only status is obtained through the existing generic
plugin route bridge. The core does not learn Telegram configuration fields or
Bot API semantics.

### CLI

The CLI builds its core registry as today, then asks the selected daemon for
plugin command contributions and merges the validated entries beneath the
plugin namespace. Parsing, usage text, `--help`, and `--help-json` use the same
merged registry, and execution calls the generic plugin action endpoint.

If the daemon is unavailable, core commands and core help still work. Dynamic
plugin commands require a reachable daemon; the error says so instead of
pretending the plugin command is unknown.

Secret fields are never accepted as ordinary flag values. A password argument
can only be read from a file or stdin, preventing the bot token from appearing
in process arguments or shell history.

### UI

Server Settings gains an Integrations section whose entries come from the same
validated plugin settings contributions. A generic React renderer displays the
declared fields, status, validation errors, and action buttons at a route scoped
to the selected server and plugin. It calls only the generic plugin route and
action APIs.

Plugins cannot inject JavaScript, HTML, styles, React components, routes, or
assets. This keeps the current build and security boundary intact while making
new settings pages possible without editing core UI code.

Password fields are write-only. The UI receives `token_configured: true|false`
and never receives the stored token or a masked value derived from it.

### Bundled plugins

`tariboy-plugin-telegram` is a separate supervised executable shipped beside
the other Tariboy binaries. Its manifest name `telegram` is reserved for the
bundled plugin. On daemon startup, the bundled manifest/executable is ensured
and enabled automatically; an operator does not install it separately.

Upgrades may replace the bundled executable and manifest but preserve its
plugin work directory. With no token configured the process remains healthy
and performs no Telegram network requests.

This mechanism is only the minimum needed to ship first-party plugins. It does
not add a plugin marketplace, arbitrary frontend packages, or an in-process
plugin SDK.

## Phase 2: Telegram plugin

### Configuration and persisted state

Configuration belongs to the Telegram plugin and is stored in its private work
directory:

- `bot_token`: write-only Bot API token;
- `allowed_uids`: normalized, sorted, duplicate-free signed 64-bit Telegram
  user IDs;
- `chat_id`: signed 64-bit Telegram supergroup ID, absent until setup succeeds.

The token file and all plugin state containing it use owner-only permissions.
The token is excluded from logs, API responses, events, diagnostics, and
support bundles. Updating a token first validates it with `getMe`; only a valid
token atomically replaces the previous value.

The plugin also persists:

- the long-polling update offset;
- the management topic ID;
- agent identity to topic ID mappings.

Configuration writes and mapping updates use atomic file replacement. A failed
token update or chat setup leaves the last working configuration intact.

An empty `allowed_uids` list is a hard deny-all state. The plugin accepts no
commands or ordinary messages, including messages in the configured chat.
While deny-all is active it continues to advance and persist the Telegram
update offset, discarding updates so that old messages cannot execute after an
allowlist is later populated.

### Operator workflow

The contributed CLI is:

```text
tariboy telegram configure --token-file FILE --allowed-uids 123,456
tariboy telegram chat setup --chat-id -1001234567890
tariboy telegram status
```

`--token-file -` reads the token from stdin. Re-running `configure` without a
token changes only the allowlist. The corresponding UI form provides a
write-only token field, the allowed UID list, status, and the same chat setup
action.

`chat setup` validates the group and bot rights, then atomically binds the chat
and creates the missing Tariboy topics. It never edits Telegram rights or forum
settings. Failure before a complete binding does not erase an earlier working
binding. Topic mappings are persisted immediately after each successful topic
creation, so retrying setup continues rather than duplicating completed work.

`status` and the UI show only safe facts: token configured, allowlist count,
bound chat ID and title, forum validation, bot topic-management permission,
polling state, and the last redacted configuration error.

### Topics

The bound supergroup contains:

- one management/server topic named `tariboyd`;
- one topic for each current agent.

The management topic is the "common server topic" and owns daemon-wide agent
and task commands. Agent topics use the agent display name as their title and a
durable agent identity as their mapping key.

At startup and on a short bounded cadence, the plugin reconciles current daemon
agents with persisted mappings and creates only missing topics. It does not add
a new daemon lifecycle-event framework. Deleting an agent does not delete or
close its topic or mapping. If Telegram reports that a mapped message thread no
longer exists, the plugin creates a replacement and updates the mapping.

### Authorization and routing

An incoming update is accepted only when all of these are true:

1. `allowed_uids` is non-empty;
2. the message has a concrete Telegram user sender whose ID is in the list;
3. its chat ID exactly matches the configured chat;
4. its message thread is the management topic or a currently mapped agent
   topic.

Anonymous administrator posts, channel senders, private chats, other groups,
the general topic, unknown threads, and unauthorized users are ignored without
a reply. Authorization is checked before command parsing or message publishing.

Bot commands may include Telegram's `@botname` suffix. Commands are handled
directly through the authenticated daemon operator API; they are never turned
into prompts for an agent.

Ordinary text is accepted only in a mapped live-agent topic. The plugin
publishes it through the existing channel bus on
`chat:telegram:<agent-name>`, with the Telegram `update_id` as the external
idempotency key. The mapped agent is subscribed through the existing channel
subscription mechanism. Existing inbox delivery, wake coalescing, processing,
reply, acknowledgement, and redelivery rules remain authoritative.

The channel sink sends only agent-produced replies for the mapped Telegram
channel and does not echo the inbound Telegram message. It acknowledges a
delivery only after Telegram accepts the send. Messages longer than Telegram's
text limit are split into ordered plain-text messages without adding a Markdown
renderer.

### Commands

The management `tariboyd` topic supports:

```text
/help
/agents
/agent create NAME IMAGE
/agent show NAME
/agent set NAME FIELD VALUE
/start NAME
/stop NAME
/kill NAME
/tasks [NAME]
/task show KEY
/task create QUEUE TITLE
/task assign KEY AGENT
/task status KEY open|in_progress|done|cancelled
/task comment KEY TEXT
```

The agent topic supports:

```text
/help
/start
/stop
/kill
/tasks
/task show KEY
/task create QUEUE TITLE
/task assign KEY
/task status KEY open|in_progress|done|cancelled
/task comment KEY TEXT
```

In an agent topic the agent argument is implicit. Task creation/assignment uses
that topic's agent where the native task operation permits it. `TITLE`, `TEXT`,
and `VALUE` consume the remaining command text so spaces do not require a
custom shell parser.

`/agent set` has an explicit safe-field allowlist: alias, image, harness, model,
effort, working directory, interactive mode, and existing Autopilot settings.
It cannot change environment variables, credentials, bot configuration, or
other secrets.

`/help` is generated from the same command registry used for dispatch and lists
every command available in that topic. Unknown commands return a short error
and `/help` hint. Operator/API validation errors are returned only in the
authorized topic that issued the command.

### Telegram transport and recovery

The plugin uses the Bot API over `net/http` with long polling; no Telegram SDK
or webhook listener is added. The next update offset is persisted after an
update is either durably accepted or intentionally discarded.

Failures are classified as follows:

- invalid/revoked token (`401`), forbidden access (`403`), non-forum chat, and
  missing topic permission are stable configuration errors; the plugin stays
  healthy and reports them without entering a process restart loop;
- rate limiting (`429`) honors Telegram's `retry_after`;
- network and transient server errors use bounded exponential backoff;
- a missing message thread triggers one topic replacement and send retry;
- no token and an empty allowlist are healthy disabled states.

No error path logs request URLs containing the token or response content that
may contain operator messages.

## Testing

Implementation is test-driven and ordered with the two phases.

Phase 1 focused tests cover manifest compatibility and validation, namespace
collision rejection, contribution discovery, normalized action invocation,
secret input handling, dynamic CLI help/execution, and generic settings form
rendering. Existing plugins must pass unchanged.

Phase 2 uses an `httptest` Bot API server and isolated daemon directories. Tests
cover token replacement, deny-all offset advancement, exact UID/chat/thread
authorization, forum and permission validation, resumable topic creation,
retained mappings, topic replacement, command dispatch, task defaults, inbound
message publication, agent reply delivery, idempotency, acknowledgement after
send, splitting, rate limiting, and redaction.

UI unit tests cover write-only token behavior, allowlist validation, safe
status rendering, and setup errors. Because the task changes the UI and bundled
Desktop payload, final verification includes `make check`, `make full-check`,
production Desktop Playwright, `tauri-driver`, `git diff --check`, and a complete
diff review. A real Telegram smoke test remains opt-in and uses an operator-owned
disposable forum supergroup; automated tests never require Telegram credentials.

## Documentation

Current documentation is updated with:

- the declarative plugin CLI/UI contribution contract and security boundary;
- bundled plugin discovery and packaging;
- Telegram setup prerequisites and Bot API limitations;
- CLI and UI workflows, command lists, deny-all semantics, topic retention,
  delivery behavior, recovery, and redaction guarantees.

## Explicit non-goals

- Creating Telegram groups or enabling forum mode.
- Promoting allowed users or changing any Telegram administrator rights.
- Deleting or closing topics when agents are deleted.
- Supporting private chats, non-forum groups, multiple groups per daemon,
  media/attachments, reactions, edits, or Telegram webhooks.
- Loading arbitrary plugin-provided frontend or CLI executable code into core
  Tariboy processes.
