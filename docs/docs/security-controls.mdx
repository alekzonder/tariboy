---
title: Security and controls
description: Trust boundaries, credential handling, budgets, audit, and operator stop controls.
sidebar:
  label: Security
  icon: shield
---

The alpha is designed for named internal partners and trusted hosts. It has no
built-in product analytics. OTLP export is disabled by default and activates
only when an operator explicitly sets `OTEL_EXPORTER_OTLP_ENDPOINT` or the
persisted `otlp_endpoint` daemon setting.

## Network boundaries

- Local and remote daemon HTTP listeners bind loopback.
- Remote access uses an SSH local-forwarding tunnel.
- System OpenSSH owns host-key verification, agent use, ProxyJump, and 2FA.
- Tariboy never disables strict host-key checking.
- Non-loopback daemon TCP requires a bearer token file.
- The tokenless loopback web listener echoes CORS only for loopback `http`
  origins, the macOS app origin `tauri://localhost`, and the Android app origin
  `http://tauri.localhost`, each matched exactly. Browsers resolve
  `*.localhost` to loopback, so the Android origin is that app's WebView or a
  loopback port-80 page, which `http://localhost` already covered. The Host
  check is unchanged.
- Every daemon HTTP server (the unix socket, daemon TCP, the loopback web
  listener, the agent tools socket, and the AI proxy) closes a connection whose
  request headers take more than 10 seconds, closes idle keep-alive
  connections after 120 seconds, and refuses headers larger than 64 KiB. The
  servers set no read or write timeout for the whole request, because SSE,
  websockets, terminals and model responses stay open for a long time.
- A command request body is limited to 1 MiB and a larger body gets HTTP 413
  `too_large`. A route raises its own limit only where it needs to: file
  uploads, queue secrets (512 KiB), and the two text-file editors (the agent
  file editor and image source files), whose limit fits their largest file
  after JSON escaping. The agent tools API limits every body to 1 MiB.
- The operator API and the agent tools API map a domain error to an HTTP
  status and code in one place, so both give the same answer. For example, an
  unknown script is 404 `not_found` and an active script is 409
  `script_active` on both. Any other error is HTTP 500 `internal` with the
  fixed text "internal error, see daemon log", and its detail goes only to the
  daemon log. A body that is not valid JSON gets 400 `bad_json` with a fixed
  text, not the text of the JSON decoder.
- Every websocket route (Tasks, message and agent-state hints, and the agent
  terminal) checks the browser `Origin` when the request has no bearer token:
  only the web listener's allowed origins above may open it, and any other
  origin gets HTTP 403 `forbidden_origin`. A request with a valid bearer token
  may come from any origin, because a federated `/terminals` page on one
  daemon opens sockets on another. A terminal websocket accepts frames of at
  most 8 MiB (one paste); a larger frame closes the socket with "message too
  big".
- The Android app permits cleartext only to `127.0.0.1`, `localhost`, and
  `::1` through its network security config, and its WebView has no Tauri IPC
  permissions. It keeps server tokens in WebView session storage, not a
  keystore.
- The Android app's native navigation guard decides every top-level WebView
  navigation. Only the app's fixed origin `tauri.localhost` (and, in a debug
  build, its configured dev server) loads inside the WebView. The page that is
  shown now never widens that. Any other
  `http` or `https` address is cancelled and given to the system browser
  through the opener plugin in a background task. The navigation callback
  returns without waiting for the browser. If opening fails, navigation stays
  cancelled and the current page stays in place. Every other scheme (`javascript:`, `file:`,
  `intent:`, and the rest) is cancelled and opens nothing. The guard never logs
  the address or raw opener error, because either can carry a token. An opener
  failure logs only the fixed message `could not open a web link in the browser`.

## Credentials

SSH private keys remain with OpenSSH and are never copied. Interactive prompt
replies live in memory only and are redacted from events. Manual HTTPS host
tokens live in macOS Keychain under service
`app.tariboy.desktop.host`, keyed by host ID. Host registry JSON contains no
tokens.

The AI proxy gives scoped short-lived tokens to harnesses/plugins and scrubs
upstream provider keys from plugin environments. Audit transcripts contain
model traffic and are sensitive operator data even though support bundles
exclude them.

Active iteration tokens and the stable loopback proxy address are carried across
a daemon restart in `~/.tariboy/aiproxy-handoff.json`. The base directory is
owner-only and the handoff file plus atomic temporary files use mode `0600`.
Tokens are never logged, audited, stored in SQLite, or included in support
bundles. Startup prunes leases that no longer map to a `running` iteration;
adoption revokes the remaining lease when its shim finishes.

## Runtime controls

- Start and Stop are the one on/off switch of an agent.
- Stopping an agent prevents new iterations.
- Kill terminates current work without stopping unrelated agents.
- Soft and hard timeouts bound an iteration.
- Budgets constrain spend.
- Agent USD budgets have independent calendar hour/day/ISO-week/month limits;
  zero is unlimited, and an exhausted request is rejected before upstream access.
- Cursor CLI traffic has no price, so it never adds to USD budgets or limits.
  Do not rely on a budget to cap a Cursor agent's spend; see
  [AI proxy](/docs/architecture/ai-proxy#cursor-harness).
- Usage and per-iteration audit make outcomes reviewable.

Removing a host is local cleanup, not remote deletion. Quitting the app leaves
daemons running. Treat both behaviors as continuity features, not stop controls.

### Codex approval mode

Each daemon chooses how its Codex agents run commands:

- `yolo` (default) starts Codex with `--yolo`: no sandbox and no approval
  prompts. Commands run with the daemon user's full file and network access.
- `approve-for-me` starts Codex with `--approve-for-me`: commands run in the
  workspace-write sandbox, and Codex reviews approval requests automatically.
  Writes outside the agent's working directory and network access from
  commands can be refused, so Tariboy tools, `git push`, or dependency
  downloads inside an iteration may fail in this mode.

The mode is per daemon, applies to every Codex agent on it, and takes effect
from the next iteration. Other harnesses are unaffected.

## Local and remote files

Local data defaults to `~/.tariboy`; runtime files default to
`~/.tariboyd`. Remote binaries live in versioned directories under
`~/.local/lib/tariboy`; managed symlinks live in `~/.local/bin`. The local
Desktop installer preflights all six command paths before changing any of them
and refuses regular files, directories, or symlinks not owned by the verified
Tariboy app bundle. The bundle has five real payload files; `ttasks` is a
managed alias whose source is `tariboy-tasks`.

Image builds resolve only the literal `$PLUGINS` root, source-relative paths,
or explicit absolute paths. Prompt paths and plugin references reject traversal
outside their roots.
Source-relative skill directories may use `../` to reference a sibling tree;
ordinary builds freeze only those explicitly declared skills alongside the
source snapshot. A source directory may itself be selected through a symlink;
symlinks inside packaged content, missing or non-regular files, and oversized
prompt files are rejected. The optional `image-creator` capability can build
from any source, skill, or prompt path readable by the daemon account; relative
source paths start at the agent's managed workdir. Grant it only to agents
trusted with that host-level read and packaging access.
Native host metadata and exported support archives use owner-only permissions.
The global `<base-dir>/global-agent-shell.sh` and per-agent
`<base-dir>/agents/<name>/agent-shell.sh` scripts are owner-only, atomically
written only after `bash -n` validation; a rejected value cannot replace the
last valid script.

User Stores clone Git sources on the selected daemon host into its own
`<base-dir>/stores/<name>/`, or use an explicitly registered local directory.
Store names and image selectors are validated; image selection rejects path
traversal and symlink escapes. Git uses the server account's existing SSH and
credential helpers without weakening host verification. Credential-bearing
URLs are rejected and subprocess output is not returned as error text.
Refresh uses fast-forward-only pull and never resets or stashes user changes.
Removing a Store preserves local source directories and built images.
Building a Store with `skills-lock.json` invokes npm's skills installer on
that host before freezing sources. For an image with `extends`, the installer
also runs once per layer with a lock, inside an owner-only temporary assembly
directory under the base directory that is removed after the build. Register and build sources you trust to
install their declared skills. Store source contents remain outside the
support-bundle allowlist.

Database backups written by nightly maintenance live in the owner-only
`<base-dir>/backups/db/` directory, and each backup file is mode `0600`. A
backup is a complete, unmasked copy of `tariboyd.db`: task text, mail,
Usage rows, and **agent and queue secret values in plaintext**. Unlike
`tariboy backup`, nothing is masked, and a deleted or rotated secret remains in
each kept backup until rotation removes it. Treat backups like the database
itself and never copy them off the host casually. Backups are outside the
support-bundle allowlist.

## Workflow scripts and queue secrets

The workflow block of an agent's Goal prompt treats its sources by trust. The
task title and description, artifact values, transition messages, and script
messages are rendered as fenced, bounded data (the fence is longer than any run
of backticks in the value, and each value is cut at a fixed length), never as
instructions; the status instructions are trusted image content, like an image
prompt. Queue secret values appear in no prompt. See
[Task workflows](/docs/task-workflows#the-goal-block).

The daemon runs the check, watch, and source scripts of a queue's
[workflow image](/docs/task-workflows#script-protocol) as the daemon account,
the same OS user as the agents. A workflow gate protects against a skipped
step, not against an agent that deliberately works around the process: an
agent can read the task directory, the unpacked image, and the run files, and
can reach the daemon socket at its default path; `tariboy tasks` uses that
socket in operator mode, so an agent that runs it sees and changes every task
as the customer. Run workflows only with agents you trust that far.

- A script receives no agent tools socket and no daemon API variables:
  `TARIBOY_TOOLS_SOCKET`, `TARIBOY_DAEMON_SOCKET`, `TARIBOY_PLUGIN_SOCKET`, and
  `TARIBOY_PLUGIN_TOKEN` are removed from its environment, and the protocol
  variables cannot be overridden by the daemon environment, an agent, the
  image's `env`, or a secret.
- A `run_as: queue` script runs in the task's owner-only state directory with
  the daemon's environment after the exports of the global shell script, the
  image's `env`, and the queue secrets; when none
  of these sets `PATH`, it gets `PATH=/usr/bin:/bin`. A `run_as: agent` check
  also receives the holder's environment and agent secrets, plus the exports of
  the global and agent shell scripts, and runs in the holder's working
  directory.
- The files of a task under `<base-dir>/tasks/<KEY>/` are owner-only:
  directories `0700`, files `0600`. The run log on disk is not redacted. The
  nightly database cleanup removes the directory of every task it deletes.
- A [source](/docs/task-workflows#sources) script runs like a `run_as: queue`
  script, with the queue secrets, in the owner-only directory
  `<base-dir>/task-queues/<QUEUE>/sources/<name>/state/`. The tasks it creates
  are authored by the customer, and their titles, descriptions, and artifact
  values are untrusted data in the Goal block like any other task's. A source
  log is readable only by the customer.

Queue secrets are stored per queue in `tariboyd.db` in plaintext, like agent
secrets. Plain queue env vars share the table but are not secret: their routes
and `ttasks queue env ls` return the values, and nothing redacts them. They are set only by the operator (`ttasks queue secret set`, value
from stdin preferred; the route accepts a body of at most 512 KiB). Removing a
secret is refused while the workflow bound to the queue requires its key; older
workflow versions still pinned by unfinished tasks are not checked, and a
script of such a task that misses its secret fails. No read route, event, task
snapshot, artifact, or result file carries a value, and Compose never handles
one. Queue secret values of at least 6 bytes are replaced with `[redacted]` in
a script's message and returned artifact values, and in the titles and
descriptions a source reports, before they are stored, and in
run log text served by the log route; every match is found before any is
replaced, so overlapping or adjacent values leave no part of either. When the
read window does not start at the beginning of the file, the daemon reads 64 KiB
more ahead of it so a secret the window start would cut is still matched, and
returns nothing from that 64 KiB lead, even where a redaction shortened the
window. A file shorter than the window plus 64 KiB is read whole, so text before
the requested window, fully redacted, may appear in it. This filter uses the queue's current values only: a
rotated or removed secret is no longer redacted in an old log, and encoded
forms are never redacted. Script authors must not print secrets. A run log is
readable only by the customer and a holder of the task, and the log of a
`run_as: agent` check only by the customer and the holder it ran as, because it
may hold that agent's own secrets, which are not redacted; an agent check
recorded while no agent held the task is the customer's alone. Neither the
database nor `<base-dir>/tasks/` nor `<base-dir>/task-queues/` is in the
support-bundle allowlist.

## Pricing catalog boundary

The daemon downloads model prices only from LiteLLM's fixed production HTTPS
URL; proxy input, model names, and operator configuration cannot redirect that
request. The client has a ten-second timeout, does not follow redirects, reads
at most eight MiB, and validates the complete catalog before publication.

The validated source document is cached as
`$TARIBOY_BASE_DIR/model-prices-litellm.json` (normally
`~/.tariboy/model-prices-litellm.json`). It is written through an owner-only
`0600` temporary file and atomically renamed. Catalog payloads and provider
credentials never enter catalog diagnostics; the corresponding logs/events
contain only a bounded source, generation time, accepted model count, and
stable error class.

The pricing cache is not part of the support-bundle allowlist and is never
collected, even when sensitive agent data is enabled. This feature does not
change loopback listener restrictions, remote bearer tokens, SSH host
verification, Keychain handling, prompt redaction, AI transcript handling, or
any other support-bundle exclusion. See [AI proxy and
audit](/docs/architecture/ai-proxy#pricing-catalog-and-request-costs) for its
runtime lifecycle.

## Alpha signing and Gatekeeper

`0.99.0` is ad-hoc signed, not Developer ID signed or notarized. Verify
`SHA256SUMS` before opening it. If Gatekeeper blocks it, Control-click only the
named `/Applications/Tariboy.app`, choose **Open**, and confirm.
If Control-click Open is unavailable, use **System Settings → Privacy &
Security → Open Anyway** for that exact app. This is a temporary scoped
exception for named alpha partners.

Do not disable Gatekeeper globally, change system-wide `spctl` policy, or remove
quarantine recursively from Applications. Developer ID signing and notarization
remain later release work.

Desktop accepts an update only when the official Tauri updater verifies the
download against the permanent public key pinned in the installed application.
The release owner's matching private key stays in GitHub Actions Secrets and
must be securely owned and backed up before an updater-enabled release. The
fixed catalog may select only a higher stable version for the supported Apple
Silicon macOS target. The WebView cannot provide a URL, signature, path, or
archive bytes.

The verified archive remains in Rust process memory and is never sent through
IPC, written to Web Storage, or added to support bundles. Closing the window to
the tray retains it; quitting the application discards it. A failed install
retains the verified archive for retry, while an interrupted download starts
again from zero. Archive memory use is proportional to the update size.

An exact `vX.Y.Z` tag triggers the repository's GitHub Actions workflow on a
macOS runner. The workflow rejects a tag that differs from either canonical
version declaration, builds and verifies the release through `make
desktop-mac`, and publishes the DMG, signed updater archive and catalog,
`SHA256SUMS`, and `release.json` with the job-scoped `contents: write`
permission. A parallel Linux job builds the Android APK, signs it with the key
from the `ANDROID_KEY*` Secrets, verifies the signature with `apksigner`, rejects
the Android debug certificate and any certificate whose SHA-256 fingerprint
differs from the `ANDROID_CERT_SHA256` repository variable, and the release is
published only when both builds succeed. The keystore Secrets are available only
to the step that writes `keystore.properties`. Before staging, the workflow rejects malformed signing packets or a
signature key identifier that differs from the pinned updater public key. This
catches an accidental wrong signing Secret; cryptographic archive verification
remains the official updater's responsibility. GitHub publication does not change
the ad-hoc-signing or Gatekeeper constraints above.

The alpha deliberately enables Tauri WebView developer tools in its
release build for local diagnosis. DevTools exposes the current WebView state
to the person operating the local Desktop session; it does not relax daemon
loopback binding, bearer-token requirements, CORS, or SSH trust boundaries.
The main Desktop WebView has the narrow Tauri window-drag permission needed by
the overlay titlebar; it does not receive resize-dragging or unrestricted
window mutation permissions.

## Support-bundle boundary

Every export selects exactly one host. The default bundle includes its raw ID
and label plus bounded platform/version/health/prerequisite state and
allowlisted lifecycle/error lines from `tariboyd.log`. The per-request
`tariboyd.access.log` is not exported. Other host records are not exported.

An explicit, unchecked sensitive-data option adds the complete redacted allowed
files for the newest 10 iterations of each agent on that host. It reads only
`result.json`, `logs/shim.log`, `logs/harness.stdout.log`, and
`logs/harness.stderr.log`; it never recursively walks an agent directory.

Redaction of agent files classifies each daemon environment variable by its
name. Values of secret-looking names (`*_TOKEN`, `*_SECRET*`, `*_API_KEY`,
`*PASSWORD*`) are removed from 4 characters, `USER`, `LOGNAME`, and `HOSTNAME`
from 3 characters, and any unknown name from 8 characters. A short allowlist of
harmless names (`SHELL`, `TERM`, `LANG`, `LANGUAGE`, `LC_*`, `PATH`, `GOPATH`,
`GOROOT`, `PWD`, `TMPDIR`, `EDITOR`, `VISUAL`, `COLORTERM`, `TERM_PROGRAM`,
`SHLVL`) keeps its values. The collector does not export the environment; an
allowlisted value that already appears in collected text is kept on purpose,
and home directory prefixes are still rewritten. A
secret-looking name is never allowlisted, and the unknown-name rule is
unchanged from policy version 1. `diagnostics.json` reports policy version 2.

The collector never includes credentials or environment values, SSH
aliases/configuration, `PROMPT.md`, context, transcripts, audit, workdirs,
configured cwd contents, global or agent shell scripts, image/plugin state,
provisioning replies, or user files. It does not open agent environment
configuration. Harness output can nevertheless contain arbitrary private natural
language, so inspect the ZIP before sending it under your organization's support
data policy. Export does not upload anything automatically.

Activity audit export is a separate, explicit operator action and is not a
support bundle. It can export one iteration or all retained iterations and may
contain prompts, reasoning, commands, tool arguments and results, model
responses, and user data. The Desktop warns before downloading the ZIP; the
export remains a local download and is never uploaded automatically. This does
not change the support-bundle exclusions above.

Portable image and team archives use a versioned manifest and bounded streaming
validation. Import rejects absolute or parent paths, NULs, duplicate/colliding
paths, symlinks, hardlinks, devices, FIFOs, excessive compressed/expanded size,
file count and path length, unmanifested content, and digest mismatches before
adopting staged data. Runnable image archives contain only the runnable image
artifact and its ref identity: never original source files or source CWD. Team
archives are compose-only and contain no image build contexts, source trees, or
runnable image bytes. They also never contain agent workspaces/data,
credentials, environment values outside the explicit compose contract,
external cwd contents, or plugin state. Raw archive routes use the same
loopback, CORS, and remote bearer-token boundary as JSON API routes.
