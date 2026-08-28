# Cross-host runnable image transfer

## Purpose

Let an operator transfer a built, non-reserved runnable image from the host
currently displaying it to one or more other ready Desktop hosts. The action
must support a selected subset or every eligible host in one operation, without
copying an image's original source directory or changing the Desktop's active
host.

## Scope and non-goals

The feature extends the Images built-image table. It reuses the existing
runnable image export and import-preview/apply APIs; it does not add a
daemon-to-daemon protocol, cross-host credentials, an archive persistence
layer, or source-image transfer. Reserved images, including `bare`, are not
transferable. A target host must already be configured and ready in Desktop.

## Architecture and data flow

`BuiltImages` already receives the explicit source `hostId`, while the
`DaemonProvider` holds the configured hosts and their current connection state.
The feature adds an **Upload to servers** action for every exportable image.

1. Opening the dialog captures the source host descriptor and image ref. It
   derives eligible targets as the local host plus every configured host whose
   state is ready, excluding the source host. Team membership is irrelevant.
2. The dialog offers individual target checkboxes and **All servers**, which
   selects every eligible target. Operators can deselect any target before
   starting. If there are no eligible hosts, the action describes that state and
   cannot start a transfer.
3. Start downloads the source archive once with `GET /api/images/{ref}/export`
   against the captured source target. The resulting `Blob` remains in React
   memory for this dialog only.
4. For each selected target, the UI uploads that same blob with
   `POST /api/image-imports`, then applies the returned preview with
   `POST /api/image-imports/{id}/apply`. Both calls are made through explicit
   `ApiTarget` values; they never read or mutate the global active daemon.
5. Completion refreshes the image list only when the captured source remains
   mounted. A destination import is immediately represented in the dialog,
   independently of a later host refresh.

No blob, import ID, host credential, progress entry, or selection is written
to localStorage or daemon persistence. Closing or unmounting the dialog drops
the blob and staged state. Cancellation prevents unstarted destinations but
cannot undo an apply already accepted by a daemon.

## UI states and accessibility

Each target has a stable row keyed by host ID and an accessible label. A row
progresses through `queued`, `exporting`, `previewing`, `importing`,
`already present`, `completed`, `failed`, or `cancelled`. The dialog contains a
status region that announces the current operation and completed target count.
Its controls are disabled while the source is exporting; after exporting,
cancellation only stops the next destination.

The destination must use the original source ref on its first apply. The
daemon's idempotent same-ref/same-digest result is rendered as **Already
present**, a successful terminal state. A ref conflict is isolated to that
host: it renders the host-provided message and a per-target **Retag and retry**
field prefilled with the source ref. Retrying reuses the in-memory blob to make
a fresh preview and apply for only that target; it does not re-export or repeat
successful targets. Other failure types are reported per target and do not
block queued hosts.

## Safety and host boundaries

All API calls retain explicit source or destination targets. The UI may not
switch the global active daemon as an implementation shortcut, and it must not
fall back to local authority if a configured host becomes unavailable. Existing
daemon archive validation remains authoritative: it enforces the bounded
portable image format, digest checks, and safe archive member rules. The
browser-mediated archive contains only a runnable artifact and digest—never an
original source CWD, agent data, SSH configuration, private key, or host token.
Remote connectivity continues through the existing loopback tunnel and system
OpenSSH trust boundary.

## Components and API helpers

- `ui/src/pages/images/BuiltImages.tsx` owns the row action and refresh signal.
- A focused transfer-dialog component owns selection, sequential progress,
  cancellation, retry, and blob lifetime.
- `ui/src/lib/api.ts` exposes `listImagesOn` and target-aware image helpers.
- `ui/src/lib/teamApi.ts` already exposes target-aware archive download,
  upload-preview, and apply calls; the implementation will use or relocate
  those helpers without changing the daemon route contract.
- `ui/src/components/DaemonProvider.tsx` supplies configured hosts. Target
  discovery filters to ready non-source hosts and includes the implicit local
  host exactly once.

## Testing

Focused React tests will prove:

1. reserved/non-exportable images do not offer the action;
2. ready non-source targets include local and configured ready hosts but omit
   the source and unready hosts;
3. **All servers** selects every eligible target and individual deselection
   remains possible;
4. one source export feeds target-specific preview/apply requests;
5. a failure on one destination does not prevent later selected destinations;
6. idempotent results are marked already present; and
7. conflict retag retry reuses the archive and only retries the failed host.

The browser fixture will cover the dialog's visible selection and per-host
progress flow. Because this changes Desktop UI behavior, final verification
also includes the production Tauri Playwright/`tauri-driver` suite with its
isolated daemon and Desktop state, plus `make full-check`. Documentation will
update the Images and Web UI guides to state the all-ready-host scope,
browser-memory archive lifetime, and explicit host-bound request rule.
