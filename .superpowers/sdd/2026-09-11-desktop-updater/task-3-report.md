# Task 3 report: global application settings and ready banner

## Outcome

Implemented the Desktop update UI contract on `improve-80-desktop-updates`.
The application now owns one route-independent update provider, an
`/app-settings` surface, a titlebar settings link, and a verified-package banner
directly below the titlebar. Updater IPC remains argument-free and independent
of daemon selection.

## Implementation

### Desktop bridge

- Added the exact typed native snapshot, including revision, current/candidate
  versions, phase, downloaded bytes, optional total bytes, and safe error text.
- Added argument-free wrappers for `desktop_update_state`,
  `desktop_update_download`, and `desktop_update_install`.
- Added the `desktop://update-state` subscription through the existing dynamic
  Tauri event bridge.
- Preserved synchronous unsubscribe semantics. The new registration callback
  fires only after the async native listener is installed; cancellation before
  registration still immediately removes the eventual listener.
- Exposed registration failure to the provider without changing existing
  daemon, host, or notification subscriptions.

### Shared provider and scheduling

- Mounted one `DesktopUpdatesProvider` around `MainApp`, so settings, route
  changes, and the global banner share one native stream and snapshot.
- Registered the event listener before requesting the current snapshot. This
  closes the listener-registration race: changes before registration appear in
  the later snapshot, while changes after registration arrive as events.
- Applied snapshots only when their revision is at least the latest accepted
  revision, preventing a delayed initial read from overwriting a newer event.
- Added one shared in-flight guard for manual checks, automatic checks, and
  installation.
- Defaulted automatic download to enabled and persisted the boolean at
  `desktop:updates:auto-download:v1`. Read and write failures are caught and
  shown without making the preference control unusable.
- Scheduled one startup check and the next check six hours after each completed
  attempt. Scheduling the next timeout after completion avoids catch-up loops
  after sleep. Disabling clears the future timeout and does not cancel an
  in-flight native request.
- Kept install explicit. UI readiness depends only on native phase `ready`.

### Settings and banner

- Added a visible, accessible `Настройки` titlebar link with accessible name
  `Настройки приложения`, separate from server Settings.
- Added `/app-settings` with a workspace return link, current Desktop version,
  `Скачивать обновления автоматически`, and
  `Проверить и скачать обновление`.
- Added accessible checking, downloading, current-version, installation, and
  error status. Unknown download totals use an indeterminate native progress
  element; percentages are shown only for a positive known total.
- Disabled actions during incompatible native phases or another in-flight
  operation. Download failures can be checked again; install failures retain
  the ready banner and its `Обновить` retry action.
- Rendered `Версия X.X.X загружена` and `Обновить` only for native phase
  `ready`. Installation progress remains visible across routes.
- Browser mode shows `Обновления доступны только в приложении Desktop.` and
  never invokes an updater command or subscribes to native events.

### Documentation

Updated the Web UI architecture document with the global settings route,
daemon independence, default six-hour automatic schedule, explicit install,
and Rust memory-only package lifetime.

## Test coverage

Added component and integration coverage for:

- one manual request while automatic downloads are disabled;
- default-enabled startup checking;
- the six-hour schedule and no catch-up loop after simulated sleep;
- disabling, persistence, and remount behavior;
- visible Web Storage write failure with an otherwise usable switch;
- listener-first reconciliation and rejection of a stale initial snapshot;
- indeterminate accessible progress when total bytes are unknown;
- signature failure without a false ready banner;
- verified ready state and explicit installation;
- failed installation with retained-package retry;
- provider and snapshot continuity across route changes;
- browser mode with no updater bridge calls;
- daemon-independent navigation to `/app-settings` from the titlebar.

## TDD evidence

### RED

Command: `make check`

Result: exit 2, as expected before production code.

- `backend-check`: passed in 25s.
- `ui-typecheck`: failed because `DesktopUpdates` and
  `DesktopUpdateSnapshot` did not exist.
- `ui-test`: failed because the new component module and titlebar link did not
  exist; 980 existing tests passed.
- UI lint, branding, and documentation checks passed.

### GREEN and failure diagnosis

The first GREEN run passed backend checks, typecheck, lint, branding, and docs,
but one of 993 UI tests made a synchronous assertion before the Radix switch
callback settled. Changing the assertion to `findByRole` tested the timing
hypothesis. The next full run waited the complete query timeout: the switch was
visibly checked but no alert appeared. That evidence disproved a delayed-alert
hypothesis and showed the spy had not replaced the VM storage object consumed by
the component. The test now installs an explicit failing Web Storage boundary
and continues to assert the user-visible alert; product code was unchanged.

Final command: `make check`

Result: exit 0.

- `backend-check`: passed in 22s.
- `ui-typecheck`: passed in 3s.
- `ui-lint`: passed in 1s.
- `ui-test`: all 993 tests passed in 94s.
- `ui-branding`: passed in 1s.
- `docs`: passed in 19s.
- `frontend-check`: passed in 118s.

## Review and limits

The complete Task 3 diff was inspected and `git diff --check` reported no
whitespace errors. No dependency, generated asset, native Rust file, daemon
target, updater URL/path argument, or version declaration changed.

Per the approved validation restriction, no standalone Vitest, browser,
`ui-dev`, native Rust, Desktop packaging, or `make full-check` command was run.
The signed macOS download/install path remains covered by the native task and
release validation rather than this UI task.
