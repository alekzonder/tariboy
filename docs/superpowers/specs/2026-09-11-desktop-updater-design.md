# Desktop application updates

Task: IMPROVE-80. Native Task comment 2059 (`Ok`) approved the design and
implementation outline in comment 2035. This records that approved contract.

## Scope

Use the official Tauri v2 Rust updater to check, download, verify, install, and
restart Desktop. Initial target: macOS 12+ Apple Silicon (`darwin-aarch64`).
Keep DMG installation. No update server, custom installer, disk cache, remote
server update action, or release version bump.

## Release and trust

Fixed endpoint:

`https://github.com/alekzonder/tariboy/releases/latest/download/latest.json`

The catalog describes the latest stable release, with version, publication
date, notes, and a `darwin-aarch64` entry containing a versioned GitHub archive
URL and signature. Use Tauri-generated `.app.tar.gz` and `.sig` outputs,
deriving filenames from the actual build. The archive includes bundled binaries.
Stage and validate the whole set before publishing, alongside the existing DMG,
SHA256SUMS, and release.json; latest must never point to an incomplete release.

Enable `bundle.createUpdaterArtifacts`. Pin the release owner's permanent public
key in application configuration. GitHub Actions Secrets
`TAURI_SIGNING_PRIVATE_KEY` and `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` supply signing.
Never commit, print, or post private keys/passwords. Missing signing material,
empty signatures, and version disagreement fail publication. No throwaway key
or placeholder may become the production trust root.

Before production updater configuration, obtain the permanent public key and
confirm secure ownership/backup of the matching private key. The release owner
configures Secrets before the first release. Key rotation is separate work:
installed applications trust their embedded key.

Updater signatures do not replace Apple signing/notarization. Existing ad-hoc
signing and Gatekeeper limitations remain. Install the first updater-enabled
version through DMG; subsequent higher signed versions use the updater.

## Native module

`desktop/src-tauri/src/updater.rs` owns one state and at most one verified
package for the process lifetime. Register it and the plugin in `main.rs`.
It is independent of daemon connectivity and selected host.

Three argument-free commands:

- `desktop_update_state`: current serializable snapshot.
- `desktop_update_download`: check the fixed catalog and download a newer
  compatible stable release, returning the final snapshot.
- `desktop_update_install`: install only the prepared package, then restart.

Snapshot fields: monotonically increasing revision, current app version, phase,
candidate version, received byte count, optional total, and safe error text
identifying the stage. Emit `desktop://update-state` on changes. The UI
subscribes and reads an initial snapshot; older revisions cannot overwrite
newer events.

| Current | Result/action | Next |
| --- | --- | --- |
| idle, up-to-date, error | check requested | checking |
| checking | no newer compatible stable release | up-to-date |
| checking | candidate found | downloading |
| checking/downloading | failure | error |
| downloading | download returns after signature verification | ready |
| ready | install click | installing |
| installing | failure | ready with error and retry |
| installing | success | restart |

Native guards reject installation without a verified package and exclude
overlapping operations. Check requests during checking, downloading, ready, or
installing cannot start another operation or replace the prepared package.
The WebView cannot supply URLs, paths, signatures, or archive bytes.
Use a finite network timeout, byte progress, and percentages only with a
nonzero known total. A transfer-complete callback precedes signature verification
in the plugin: only successful return from `download()` authorizes ready.
Reject prereleases and non-increasing versions.

Keep verified bytes in Rust memory; do not send them through IPC or persist
readiness in localStorage. Closing to tray retains the package; full exit loses
it. Memory is proportional to archive size, and interrupted downloads restart
from zero. A ready package is not replaced by background checks.

Install off the UI thread using the same verified package. On macOS the running
process replaces the bundle through the updater, then calls Tauri restart.
Installation failure retains retry and never restarts. Installation requires a
writable installed location, not a read-only DMG, and may prompt for system
authorization. Do not promise transactional rollback or recovery from failure
to start the new application.

## Settings and banner

Add `/app-settings`, independent of `/servers/:hostId/settings`, accessible
from a labeled settings action beside the titlebar theme action, including
without any daemon. Reuse existing Button, Switch, Card, and layout styles.
Provide navigation back to the workspace.

Display current Desktop version, the switch
`Скачивать обновления автоматически`, the manual action
`Проверить и скачать обновление`, progress, current-version result, and
retryable errors. Automatic download defaults to true and persists as a boolean
under `desktop:updates:auto-download:v1`. Handle unavailable Web Storage
without crashing and report persistence failures.

One route-independent provider schedules a check on startup and every six
hours while enabled. After sleep, request at most one due check, without
catch-up loops. Disabling prevents future automatic checks; an in-flight
download finishes. Manual checking works when automatic downloads are off.
Never automatically install.

Below the titlebar, above content, show `Версия X.X.X загружена` and
`Обновить` only for a verified ready package. Keep progress/errors accessible,
disable duplicate actions, and expose installation progress. Browser-only mode
does not call GitHub and explains that Desktop is required, without false
success.

## Daemon lifecycle

The updater replaces the Desktop bundle. It neither updates remote servers nor
stops agents. Preserve existing shutdown tunnel cleanup and startup
reconnection/local bundled-version reconciliation. Do not promise the next
startup performs no daemon lifecycle actions.

## Validation and workflow

Only `make check` is authorized for verification. Do not run `make full-check`,
standalone Cargo tests/clippy, browser/e2e, or actual macOS update checks.
Observe release/UI RED/GREEN through `make check`. Rust tests can be written,
but are unexecuted under this restriction: `make check` neither compiles nor
tests the Rust host or the update path between two signed macOS builds.

Never test against the live base/runtime directories or listener. Inspect the
complete diff and whitespace. Internal spec/plan-only edits require Markdown
and diff inspection, not product documentation build gates.

With implementation, update development, web-ui, security, support, and release
runbook documentation. Open one PR from `improve-80-desktop-updates`, record it
on the Native Task, set Wait customer, and start durable monitoring. Do not
merge. Close the task only after observed merge, post-merge checks, and cleanup.
