# Streaming File Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upload Desktop and CLI files up to 1 GiB with bounded streaming memory.

**Architecture:** Add a hand-written raw-body API route and a shared confined disk writer. Keep the legacy JSON/base64 command at 16 MiB, while Desktop and `tariboy cp` use the new route directly.

**Tech Stack:** Go `net/http`, `io`, and `os.Root`; React/TypeScript Fetch API; Vitest; Go testing.

**Spec:** `docs/superpowers/specs/2026-09-08-streaming-file-upload-design.md`

## Global Constraints

- Maximum raw file size is exactly `1 << 30` bytes.
- Existing `PUT /api/files` remains JSON/base64 compatible and limited to `16 << 20` decoded bytes.
- Preserve loopback/authentication boundaries, `os.Root` confinement, owner-only permissions, response envelope, and explicit daemon targeting.
- Add no dependency and do not bump the product version.
- Run `make check`, never `make full-check`, for final verification.

---

### Task 1: Streaming daemon upload

**Files:**
- Create: `internal/api/file_upload.go`
- Modify: `internal/api/server.go`
- Modify: `internal/commands/files.go`
- Test: `internal/api/server_test.go`
- Test: `internal/commands/files_test.go`

**Interfaces:**
- Produces: `api.SaveUploadedFile(baseDir, name string, body io.Reader, maxBytes int64) (api.UploadResult, error)`.
- Produces: `PUT /api/files/raw?name=<url-encoded-name>` returning the standard envelope around `UploadResult`.
- Preserves: registry command `files.upload` at `PUT /api/files` with a 16 MiB decoded limit.

- [x] **Step 1: Write failing raw-route tests**

Add tests that submit an 18 MiB streaming reader to `/api/files/raw`, verify
the file bytes/permissions/result, reject a declared size above 1 GiB without
reading, and call `SaveUploadedFile` with a small limit to prove an
unknown-length overflow removes its partial directory.

- [x] **Step 2: Run the daemon tests to verify RED**

Run: `go test ./internal/api ./internal/commands -run 'Upload|CpShared' -count=1`

Expected: FAIL because `/api/files/raw` and `SaveUploadedFile` do not exist.

- [x] **Step 3: Implement the minimum streaming writer and route**

Implement one `io.LimitReader(body, maxBytes+1)` copy into an `os.Root`-opened
`0600` file under a unique `0700` directory. Validate the filename before
filesystem mutation and defer cleanup until copy and close succeed. Register
the raw route in `Server.Handler`, reject oversized declared bodies first, map
`api.UserError` to its status/code, and write `UploadResult` with `WriteOK`.

Set the legacy handler back to `16 << 20`; decode base64 as before and pass a
`bytes.NewReader(raw)` to `SaveUploadedFile`.

- [x] **Step 4: Run the daemon tests to verify GREEN**

Run: `go test ./internal/api ./internal/commands -run 'Upload|CpShared' -count=1`

Expected: PASS.

---

### Task 2: Streaming UI and CLI clients

**Files:**
- Modify: `ui/src/lib/api.ts`
- Test: `ui/src/lib/api.test.ts`
- Modify: `internal/client/client.go`
- Modify: `internal/commands/files.go`
- Test: `internal/commands/files_test.go`

**Interfaces:**
- Consumes: `PUT /api/files/raw?name=<url-encoded-name>`.
- Produces: `client.UploadFile(route string, body io.Reader, size int64) (json.RawMessage, error)`.
- Preserves: `client.Upload` as the compose gzip POST helper.

- [x] **Step 1: Write failing client contract tests**

Change the UI test to require a `PUT` whose body is the exact `File`, whose URL
contains the encoded name, and which never calls `arrayBuffer()`. Change the CLI
test server to require the raw path, octet-stream content type, declared file
size, and exact request bytes.

- [x] **Step 2: Run client tests to verify RED**

Run: `go test ./internal/commands -run 'CpShared' -count=1 && cd ui && npm test -- src/lib/api.test.ts`

Expected: FAIL because both clients still create JSON/base64 bodies.

- [x] **Step 3: Implement minimum streaming clients**

Make `serverUploadFile` pass the `File` to `apiRawOn`, decode the successful
standard envelope, and remove `fileBase64`. Add a private raw-request helper in
the Go client, keep `Upload` as its POST/gzip wrapper, and add `UploadFile` as
its PUT/octet-stream wrapper. In `cpCommand`, replace `os.ReadFile` with
`os.Open`, `Stat`, and `UploadFile`.

- [x] **Step 4: Run client tests to verify GREEN**

Run: `go test ./internal/commands ./internal/client -run 'Upload|CpShared' -count=1 && cd ui && npm test -- src/lib/api.test.ts`

Expected: PASS.

---

### Task 3: Current documentation and final verification

**Files:**
- Modify: `docs/docs/architecture/index.mdx`
- Modify: `docs/docs/architecture/web-ui.mdx`
- Modify: `docs/docs/binaries/operator-cli.mdx`
- Modify: `docs/docs/tasks.mdx`
- Modify: `docs/docs/reference/commands.md`

**Interfaces:**
- Documents: raw 1 GiB UI/CLI flow and retained 16 MiB JSON/base64 compatibility route.

- [x] **Step 1: Update current product documentation**

Describe `PUT /api/files/raw` as the path used by Desktop and `tariboy cp`, the
1 GiB streaming bound and partial-file cleanup, and the retained 16 MiB limit
for `files upload`/legacy JSON clients.

- [x] **Step 2: Run focused suites**

Run: `go test ./internal/api ./internal/commands ./internal/client -count=1`

Run: `cd ui && npm test -- src/lib/api.test.ts src/components/SendFilesButton.test.tsx src/components/TuiScreen.test.tsx`

Expected: PASS.

- [x] **Step 3: Run the final branch gate**

Run: `make check`

Run: `git diff --check`

Expected: both exit 0. Do not run `make full-check`.

- [x] **Step 4: Inspect and commit**

Inspect the complete diff, confirm no generated Desktop or unrelated files are
present, and commit the implementation and documentation with a focused
message.
