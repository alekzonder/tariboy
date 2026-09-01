# Mutable Agent Image Tags Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild ordinary image tags safely, publish several tags from one CLI command, and activate changed mutable refs at the next agent iteration boundary.

**Architecture:** Mark only ordinary authoring refs as mutable, atomically retain their prior archives by digest before replacement, and reuse the existing pending-image launch gate for activation. Extend the registry CLI parser only with a repeatable-argument bit; all immutable publication paths stay unchanged.

**Tech Stack:** Go 1.26, SQLite, filesystem-backed tar.gz image store, Markdown/MDX documentation.

**Spec:** `docs/superpowers/specs/2026-09-01-mutable-agent-image-tags-design.md`

## Global Constraints

- Never test against live `~/.tariboy`, `~/.tariboyd`, or `127.0.0.1:9990`.
- `basic` and `bare` stay daemon-managed; controlled-improvement release refs stay immutable.
- Running iterations are never interrupted; image changes use the existing launch gate.
- No version bump is part of this ordinary feature task.

---

### Task 1: Mutable image-store generations

**Files:**
- Modify: `internal/image/store.go`
- Modify: `internal/image/store_ops.go`
- Modify: `internal/image/build.go`
- Modify: `internal/image/build_v2.go`
- Test: `internal/image/build_test.go`
- Test: `internal/image/build_v2_test.go`

**Interfaces:**
- Consumes: existing `Store.Inspect`, `Store.InspectPinned`, `Store.UnpackPinned`, `Build`, and `BuildV2` behavior.
- Produces: `WithMutableRef()` for schema v1, `BuildV2Mutable(...)` for schema v2, and digest-retained mutable generations readable through the existing pinned APIs.

- [ ] **Step 1: Write failing mutable-generation tests**

Add one schema-v1 and one schema-v2 test that build changed source twice at the
same ref through the mutable entry point, assert the current digest changes,
and assert both `InspectPinned(ref, oldDigest)` and
`UnpackPinned(ref, oldDigest, dir)` still resolve the first archive. Keep the
existing immutable `Build`/`BuildV2` duplicate-ref assertions unchanged.

- [ ] **Step 2: Run the focused tests and confirm RED**

Run: `go test ./internal/image -run 'Mutable|Pinned'`

Expected: FAIL because mutable build entry points and ordinary retained history
do not exist.

- [ ] **Step 3: Add the minimum mutable publish path**

Add an internal publish option used by both archive writers. Before replacing
an existing mutable ref, hard-link its validated current archive to a
digest-addressed history path, then rename the complete temporary archive over
the ref and write a small mutable marker. Generalize pinned inspection and
unpacking to consult that history only for marked mutable refs, preserving the
existing `.managed` compatibility path for reserved images.

The public shape is:

```go
func WithMutableRef() BuildOption
func BuildV2Mutable(src *imagefile.V2, roots imagefile.ResolveRoots, ref Ref,
    store *Store, clock func() time.Time,
    external plugincaps.ExternalResolver) (Manifest, error)
func (s *Store) IsMutable(ref Ref) bool
```

- [ ] **Step 4: Run GREEN and commit**

Run: `go test ./internal/image`

Commit: `feat: retain mutable image tag generations`

---

### Task 2: Repeatable CLI tags and protected rebuilds

**Files:**
- Modify: `internal/registry/registry.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/commands/image.go`
- Test: `internal/cli/cli_test.go`
- Test: `internal/commands/image_test.go`

**Interfaces:**
- Consumes: `WithMutableRef()`, `BuildV2Mutable(...)`, image provenance/snapshot stores, and the `image_releases` table.
- Produces: `registry.Arg{Repeatable: true}`, a `tag` parameter represented as `[]string` for repeated CLI flags, and an image-build response containing per-tag results.

- [ ] **Step 1: Write failing CLI and command tests**

Add a CLI test that runs:

```go
[]string{"image", "build", "--name", "reviewer", "--path", source,
    "--tag", "latest", "--tag", "v2"}
```

and asserts the request body contains `tag: []string{"latest", "v2"}`. Add
command tests proving a changed `latest` rebuild succeeds, both requested refs
exist, duplicate/reserved tags fail before publication, and a ref present in
`image_releases` is rejected as immutable.

- [ ] **Step 2: Run the focused tests and confirm RED**

Run: `go test ./internal/cli ./internal/commands -run 'Repeatable|MultipleTags|Rebuild|Release'`

Expected: FAIL because repeated scalar flags currently overwrite one another
and image build still uses immutable publication.

- [ ] **Step 3: Implement repeatable parsing and multi-tag build**

Add `Repeatable bool` to `registry.Arg`. In `parseArgs`, append converted values
only for repeatable args and retain last-value behavior for every existing
scalar flag. Mark only `image.build`'s `tag` argument repeatable. Coerce HTTP
`tag` input from either a string or string array, default to `latest`, validate
all refs and duplicates first, parse the source once, and publish each through
the mutable builder. Preserve the current single-result fields for one tag and
add an `images` array for multiple tags.

Before replacing an existing ref, query `image_releases` by `image_ref`; return
`immutable_release` when present. Reserved refs keep `reserved_image`.

- [ ] **Step 4: Run GREEN and commit**

Run: `go test ./internal/cli ./internal/commands`

Commit: `feat: build agent images with repeatable tags`

---

### Task 3: Next-iteration mutable-ref activation

**Files:**
- Modify: `internal/loop/image_activation.go`
- Test: `internal/loop/image_activation_test.go`

**Interfaces:**
- Consumes: `Store.IsMutable`, `Store.Inspect`, `Store.InspectPinned`, `agent.Store.SetPendingImage`, and the existing `activatePendingImage` promotion path.
- Produces: automatic staging of a changed active mutable ref when no explicit pending assignment exists.

- [ ] **Step 1: Write the failing launch-gate regression test**

Create an isolated agent on the first digest of a mutable ref, replace that ref,
and assert the agent row and unpacked image remain on the first digest before
`activatePendingImage`. Call the gate once and assert the second digest is now
active, the old digest remains pinned-readable, and the activation audit names
both digests. Add a sibling case proving an immutable ref remains pinned.

- [ ] **Step 2: Run the focused test and confirm RED**

Run: `go test ./internal/loop -run 'MutableRef|ImmutableRefRemainsPinned'`

Expected: FAIL because an empty pending assignment currently inspects only the
agent's old pinned digest.

- [ ] **Step 3: Reuse the pending activation path**

When pending is empty and the active ref is mutable, inspect the current ref.
If its digest differs, call `SetPendingImage(agent, ref, currentDigest)`, assign
that value to the local pending variable, and continue through the existing
staging/promotion code. If unchanged or immutable, retain the current early
return.

- [ ] **Step 4: Run GREEN and commit**

Run: `go test ./internal/loop`

Commit: `feat: activate mutable image refs between iterations`

---

### Task 4: Product documentation and complete verification

**Files:**
- Modify: `README.md`
- Modify: `docs/docs/images/index.mdx`
- Modify: `docs/docs/architecture/state-model.mdx`
- Modify: `docs/docs/architecture/iteration-loop.mdx`

**Interfaces:**
- Consumes: the implemented CLI, store, and launch-gate behavior.
- Produces: current operator and architecture documentation for mutable authoring refs and repeatable tags.

- [ ] **Step 1: Update current documentation**

Replace the blanket ordinary-ref immutability statement with the exact split:
ordinary `image build` refs are mutable with retained digest generations;
imports, registry artifacts, reserved refs, and controlled releases remain
immutable. Document repeated `--tag`, the per-tag result, and activation only at
the next launch gate.

- [ ] **Step 2: Run documentation and repository checks**

Run:

```bash
(cd docs && npm run doctor && npm run build)
make check
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Inspect the full diff and commit**

Run: `git diff --stat && git diff`

Confirm no generated UI output, version pins, credentials, or unrelated edits
are present.

Commit: `docs: describe mutable agent image tags`
