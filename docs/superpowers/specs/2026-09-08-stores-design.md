# Per-daemon Stores

IMPROVE-33. Approved in Native Task comments #1612, #1616, and #1619.
Completion mode: PR. Verification: `make check` only; never `make full-check`.

## Ownership

Each tariboyd persists only unique name and source in its SQLite database.
Git HTTPS/SSH/scp-style sources clone to `<absolute base-dir>/stores/<name>/`.
Default example: `/home/agent/.tariboy/stores/team/`; registration database:
`/home/agent/.tariboy/tariboyd.db`. Local absolute directories are used in
place on the selected server. Built-in `store/versions/` is separate.

## Inventory and lifecycle

Read `images/<image_name>/Tariboyfile.yaml` on every view with the existing
parser; image_version determines the version, absent means latest.
Missing images directory means empty inventory. Report individual malformed
images without hiding siblings. Register only after successful clone or local
directory validation. Refresh runs `git pull --ff-only` for clones and local
Git checkouts; non-Git directories are reread. Never reset or stash.
Removal deletes registration and only its own managed clone; local sources
and built images remain intact.

## Build

`tariboy image build <store_name>/<image_name>` resolves on the daemon and
uses the existing ordinary image builder. Output defaults to image_name and
image_version; --name and --tag override them. Existing --path builds remain
compatible. A selector and explicit path are mutually exclusive.
Before freeze, run `npx skills experimental_install` at the Store root if
skills-lock.json exists, then at the image directory if it has its own lock.
Installation failure stops publication. Serialize Store mutation and build
preparation so refresh cannot race source freeze. Keep existing snapshot,
provenance, immutable-ref protection, and atomic publication.

## Surfaces

Shared registry commands: store add <name> <source>, store list,
store show <name>, store refresh <name>, store remove <name>.
CLI and UI use the same server implementation. Stores is beside Images in
explicit server navigation. List supports name/source creation. Detail shows
source, images and versions, individual errors, Refresh, Remove, Build, and
pending/failure states. All requests retain the route-selected host.
Built results appear in Images. Label current content Images; future skills
and plugins do not get placeholder buttons.

## Boundaries

Validate name components and reject selectors escaping images, including
symlinks. Invoke Git and npx with argument arrays and bounded contexts.
Private Git uses server OpenSSH and credential helpers, without a token store.
Preserve host verification. Reject credential-bearing URLs and do not return
raw subprocess output that could reveal credentials.
Tests isolate base/runtime directories and never use the live daemon.
No product version bump or native host change.

## Acceptance and verification

Cover local and Git sources, failed clone and duplicates, uncached inventory,
empty and malformed images, refresh fast-forward/conflict, safe removal,
versions, lock installation ordering/failure, unsafe selectors, old path
build compatibility, and route-selected host UI operations.
Update current docs with paths, commands and Git/npm prerequisites.
Run make check; open exactly one PR and monitor without merging it.
