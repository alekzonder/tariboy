# Plugin Operator Surfaces Implementation Plan

> Execute before the Telegram plugin plan. Keep protocol version 1 and reuse
> the existing plugin action/route bridge.

**Goal:** Let a plugin declaratively add namespaced CLI commands and a generic
server-settings form without loading plugin code into core processes.

**Architecture:** Validate command/settings descriptors in `plugin.json`, expose
them through one authenticated daemon command, convert command descriptors into
CLI-only registry entries, and render settings descriptors with existing React
form primitives. Secret CLI inputs are file/stdin-only and plugin actions remain
the only mutation path.

**Tech stack:** Go stdlib, existing command registry/plugin host, React/TypeScript,
Vitest/Testing Library.

## Task 1: Manifest contribution contract

**Files:**

- Modify: `internal/plugins/manifest.go`
- Test: `internal/plugins/manifest_test.go`

1. Add failing table tests for a valid namespaced command/settings declaration,
   legacy manifests, invalid paths/actions/types, duplicate command paths/field
   names, and secret fields without file/stdin input.
2. Add the minimal descriptor structs and validation. Command paths are relative
   to the plugin namespace; supported argument types are string, integer,
   integer-list, boolean, and secret-file. Settings support string, password,
   integer-list, status, and action.
3. Run `go test ./internal/plugins -run 'Manifest|Contribution'`.
4. Commit the focused manifest change.

## Task 2: Contribution discovery API

**Files:**

- Modify: `internal/registry/registry.go`
- Modify: `internal/plugins/host.go`
- Modify: `internal/commands/plugin.go`
- Modify: `internal/commands/daemon.go`
- Test: `internal/plugins/host_test.go`
- Test: `internal/commands/plugin_test.go`

1. Add failing tests that discovery returns validated contributions for enabled
   installed plugins, omits disabled plugins and secrets, and rejects an invalid
   plugin name/action payload.
2. Add `Contributions()` to `registry.PluginControl` and `plugins.Host`, reading
   the already-installed validated manifests and returning stable sorted data.
3. Register `plugin.contributions` at `GET /api/plugin-contributions`.
4. Keep mutations on `POST /api/plugins/{name}/action`; validate a contributed
   action's submitted fields before forwarding it and preserve existing generic
   `plugin action` compatibility.
5. Run `go test ./internal/plugins ./internal/commands`.
6. Commit the discovery API change.

## Task 3: Dynamic CLI registry

**Files:**

- Modify: `internal/registry/registry.go`
- Modify: `internal/cli/cli.go`
- Add: `internal/cli/plugins.go`
- Modify: `cmd/tariboy/main.go`
- Test: `internal/cli/cli_test.go`
- Test: `cmd/tariboy/main_test.go`

1. Add failing tests for merged `telegram` group help, `--help-json`, action
   payload normalization, collision rejection, daemon-unavailable fallback, and
   secret loading from a `0600` file or stdin without argv exposure.
2. Add a small CLI-only plugin invocation descriptor to registry commands.
3. Fetch `/api/plugin-contributions` before normal dispatch, merge valid commands
   beneath the plugin namespace, and route invocation through the existing
   generic action endpoint.
4. Read `secret-file` arguments inside CLI dispatch, remove their path from the
   forwarded data, and reject non-regular or overly permissive token files.
5. Preserve all core commands/help when discovery is unavailable; report daemon
   unavailability when an unresolved command could only be dynamic.
6. Run `go test ./internal/cli ./cmd/tariboy`.
7. Commit the CLI change.

## Task 4: Generic plugin settings UI

**Files:**

- Modify: `ui/src/lib/api.ts`
- Add: `ui/src/pages/settings/PluginSettings.tsx`
- Add: `ui/src/pages/settings/PluginSettings.test.tsx`
- Modify: `ui/src/pages/settings/SettingsPage.tsx`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/App.test.tsx`

1. Add failing component tests for contribution discovery, Integrations nav,
   string/integer-list/password inputs, write-only password behavior, status,
   action submission, and server-target routing.
2. Add typed API helpers for contributions, plugin routes, and plugin actions.
3. Add one generic settings renderer using existing `Input`, `Label`, and
   `Button`; do not add a plugin component registry or HTML injection.
4. Populate Integrations from discovery and add
   `/servers/:hostId/settings/integrations/:plugin`.
5. Run `npm test -- --run PluginSettings App` and `npm run typecheck` in `ui/`.
6. Commit the UI change.

## Task 5: Bundled plugin installation seam

**Files:**

- Add: `internal/plugins/bundled.go`
- Add: `internal/plugins/bundled_test.go`
- Modify: `internal/daemon/daemon.go`

1. Add failing tests that a sibling bundled executable is copied into the
   versioned plugin directory, enabled, upgraded without replacing `workdir`,
   and safely skipped when absent in a developer build.
2. Implement one `EnsureBundled` helper using the existing install/store layout
   and atomic rename. Reserve the bundled name and keep executable resolution
   inside the plugin directory.
3. Call it before `Host.StartAll`; no generic marketplace or SDK.
4. Run `go test ./internal/plugins ./internal/daemon`.
5. Commit the bundled seam.

## Task 6: Architecture documentation and phase verification

**Files:**

- Modify: `docs/docs/plugins/index.mdx`
- Modify: `docs/docs/architecture/web-ui.mdx`
- Modify: `docs/docs/binaries/index.mdx`

1. Document the manifest schema, namespace/security boundary, dynamic CLI
   availability, generic settings renderer, and bundled lifecycle.
2. Run `gofmt` only on changed Go files, then `make check`.
3. Run `git diff --check` and inspect the complete phase diff.
4. Resolve all Critical/Important review findings before starting Telegram.
