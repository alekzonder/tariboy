# tbox Stage 2 (Stores and Remote Sources) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `tbox` registers Stores (git, https archive, directory), lists their images, skills and prompts, and builds images whose skills and prompts come from git or https addresses, pinned by a lock file; schema v2 Store images that rely on `skills-lock.json` build without Node.

**Architecture:** Two network primitives (`gitx`, `fetch`) sit under a `source` resolver that turns a remote entry into a directory in `$TBOX_HOME/cache`. `build` asks the resolver for every `git` / `url` entry and records the result in `tbox-lock.json`; the fingerprint reads only the lock, so an unchanged source needs no network. A `stores` package keeps the registry and the inventory; commands address Store content as `<store>/<name>`.

**Tech Stack:** Go 1.26, standard library, system `git`, `gopkg.in/yaml.v3`, `golang.org/x/text` (collation for the `skills-lock.json` hash only).

**Spec:** `docs/superpowers/specs/2026-11-16-tbox-design.md` (sections «Stores», «Tariboyfile schema v3», «Сборка», «Пересборка только при изменении»). Stage 1 plan: `docs/superpowers/plans/2026-11-16-tbox-stage-1-core.md`; its Global Constraints apply here unchanged.

## Global Constraints

- Everything in the stage 1 Global Constraints, plus the following.
- No test uses the network. Git tests create repositories with `git init` under `t.TempDir()`; HTTP tests use `httptest.NewTLSServer` with its client injected. Git runs in tests with `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`, and author/committer identity set through environment variables.
- `git` is always invoked with an argument array, a context timeout, `GIT_TERMINAL_PROMPT=0`, and `-c core.hooksPath=/dev/null`. A URL with user-info (`https://user:pass@host/...`) is rejected before git runs. A ref that is empty-after-trim, starts with `-`, holds `..`, a space or a control character, or is longer than 255 characters is rejected before git runs.
- Remote URLs: `https://` for archives and single prompt files; for git also `ssh://`, scp-style `user@host:path`, and `owner/repo` (expanded to `https://github.com/owner/repo.git`). `http://`, `file://`, `ext::` and local paths are rejected in production; the primitives take an `AllowLocal bool` that tests set.
- Archive extraction: regular files and directories only, no symlinks or hardlinks, no absolute or `..` paths, at most 4096 members, 64 MiB compressed, 256 MiB expanded, 512-byte paths. A single top-level directory is stripped.
- tbox never writes into a Store's directory and never into an image source directory, except `tbox-lock.json` next to the Tariboyfile when that directory is writable and is not inside `$TBOX_HOME/stores`.
- The fingerprint check never uses the network. Only `tbox build --update`, `tbox store refresh`, `tbox store add` and the first resolution of an entry that has no lock record may use it.
- Error messages never include raw subprocess output that could carry credentials; they include the command name, the address with user-info stripped, and git's last stderr line with any `://user:pass@` masked.

## Review Focus

1. A git address or ref crafted as an option (`--upload-pack=...`, `-oProxyCommand=...`) or a `path` with `..`: rejected before git runs; nothing outside the cache is read. (Tasks 1, 2)
2. An archive with a symlink member, a `../` member, or a declared size over the limit: rejected, nothing extracted outside the staging directory. (Task 1)
3. The network is down and the lock and cache are present: `tbox build` and `tbox run` work and report "up to date". The network is down and the cache is empty: a clear error naming the address, not a hang. (Task 2)
4. A Store whose one image has a broken Tariboyfile or one skill has a bad `SKILL.md`: listing shows the others and the error for that one. (Task 4)
5. Two Stores that both hold a skill named `x`: `tbox skills` shows both as `<store>/x`; a bare `x` outside a session is an error that lists the candidates. (Task 5)

---

### Task 1: Network primitives — `gitx` and `fetch`

**Files:**
- Create: `tbox/internal/gitx/gitx.go`, `validate.go`, `gitx_test.go`, `tbox/internal/fetch/fetch.go`, `extract.go`, `fetch_test.go`

**Interfaces:**
- Produces:
  - `gitx.Git{Timeout time.Duration; AllowLocal bool; Env []string}` (zero value usable; default timeout 120 s).
  - `gitx.NormalizeURL(s string, allowLocal bool) (string, error)` — applies the URL rules above; returns the canonical URL.
  - `gitx.CheckRef(ref string) error`.
  - `(Git).Mirror(ctx, url, dir string) error` — `dir` absent: `git clone --mirror`; present: `git remote update --prune`.
  - `(Git).Resolve(ctx, mirrorDir, ref string) (commit string, err error)` — empty ref: the remote default branch; otherwise branch, then tag, then a 7-40 hex commit id; returns the full 40-hex id; `gitx.ErrRefNotFound` otherwise.
  - `(Git).Export(ctx, mirrorDir, commit, subpath, dest string) error` — writes the tree (or the subtree at `subpath`) of `commit` into the new directory `dest` through `git archive` piped into the safe extractor; `subpath` is cleaned and must not escape.
  - `(Git).ExportFile(ctx, mirrorDir, commit, path string, max int64) ([]byte, error)` — `git show <commit>:<path>` with a size cap.
  - `(Git).Clone(ctx, url, ref, dir string) (commit string, err error)` and `(Git).Pull(ctx, dir, ref string) (commit string, err error)` — a working clone for git Stores: checkout of the resolved commit, detached; never `reset --hard` over local changes (a dirty tree is an error).
  - `fetch.Client{HTTP *http.Client; Timeout time.Duration; AllowInsecure bool}`.
  - `(Client).Get(ctx, url string, max int64) ([]byte, error)` — https only unless `AllowInsecure`; rejects user-info; follows at most 5 redirects, each re-validated.
  - `fetch.ExtractArchive(data []byte, dest string) error` — `.tar.gz` or `.zip` detected by content; limits above; `dest` must not exist; staged in a sibling temp directory and renamed.

- [ ] **Step 1: Tests.** gitx: `NormalizeURL` table (accepts `https://github.com/a/b.git`, `git@github.com:a/b.git`, `ssh://git@h/a/b`, `a/b` → `https://github.com/a/b.git`; rejects `https://u:p@h/a`, `http://h/a`, `file:///x`, `ext::sh -c x`, `--upload-pack=x`, `/abs/path` unless `allowLocal`); `CheckRef` table from the constraints; with a temp repo holding two commits, a branch `dev` and a tag `v1`: `Mirror` then `Resolve` for `""`, `dev`, `v1`, a 7-char commit, and `nope` (`ErrRefNotFound`); `Export` of subpath `skills/a` yields exactly that subtree with the executable bit kept; `Export` with subpath `../x` fails; `ExportFile` over the cap fails; `Clone` at `v1` then `Pull` to `dev` moves the checkout; `Pull` on a dirty tree fails and leaves the file untouched; a mirror URL that does not exist fails within the timeout and the error text holds no user-info. fetch: `Get` from a TLS test server; over-size body fails; `http://` rejected; redirect to `http://` rejected; `ExtractArchive` for tar.gz and zip with a single top directory stripped; rejects symlink member, `../x`, absolute path, 4097 members, expanded size over the cap; existing `dest` fails.
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3:** `make tbox-check`. Commit `tbox: add git and https primitives for remote sources`.

### Task 2: Lock file, source resolver, remote entries in `build`

**Files:**
- Create: `tbox/internal/lock/lock.go`, `lock_test.go`, `tbox/internal/source/source.go`, `source_test.go`
- Modify: `tbox/internal/tfile/resolve.go` (remote entries no longer resolved here), `tbox/internal/build/build.go`, `fingerprint.go`, `build_test.go`, `tbox/internal/cli/cmd_build.go` (`--update`)

**Interfaces:**
- Consumes: `gitx`, `fetch`, `tfile`, `paths.Home`.
- Produces:
  - `lock.Filename = "tbox-lock.json"`; `lock.Entry{Kind string /* git|url */; URL, Path, Ref, Commit, SHA256 string}`; `lock.File{Version int; Entries map[string]Entry}`; `lock.Key(kind, url, path, ref string) string`; `lock.Read(path string) (File, error)` (missing file is an empty lock); `(File).Write(path string) error` (atomic, sorted keys, trailing newline).
  - `lock.PathFor(sourceDir string, home paths.Home) string` — `<sourceDir>/tbox-lock.json` when the directory is writable and not under `home.StoresDir()`, else `<home>/locks/<sha256 of absolute sourceDir>.json`.
  - `source.Resolver{Home paths.Home; Git gitx.Git; HTTP fetch.Client}`.
  - `(Resolver).SkillDir(ctx, e tfile.SkillEntry, pinned *lock.Entry) (dir string, got lock.Entry, err error)` and `(Resolver).PromptFile(ctx, e tfile.PromptEntry, pinned *lock.Entry) (path string, got lock.Entry, err error)`.
  - Cache layout: git mirrors at `<home>/cache/git/<sha256(url)>`; exported trees at `<home>/cache/sources/<sha256(key + commit)>/`; url archives at `<home>/cache/sources/<sha256(url) + "-" + sha256(body)[:16]>/`; prompt files as a single file in such a directory.
  - With `pinned` set and its cache directory present: no network. With `pinned` set and the cache missing: fetch exactly the pinned commit (git) or re-download and require the pinned `SHA256` (url); a different hash is an error that says the remote content changed and names `tbox build --update`. Without `pinned`: resolve the ref to a commit (git) or download (url) and return the new entry.
  - `Entry.SHA256` for a skill directory is the skill's `tree_sha256` (from `skill.Prepare`); for a file it is the body hash.
  - `build.Options` gains `Update bool` and `Resolver *source.Resolver` (nil: remote entries fail with a clear error).
  - Skill metadata for a remote skill: `source` is `git:<url>#<path>@<commit>` or `url:<url>`; `category` is a value tariboy's `validSkillCategory` accepts (use `source`). Prompt layers likewise.
  - Fingerprint: for each remote entry, its lock key plus `Commit` and `SHA256` from the lock; an entry with no lock record makes the fingerprint "unknown", which always builds.
  - `tbox build ... --update` re-resolves every remote entry, rewrites the lock, and reports each entry whose commit or hash changed as `updated <key> <old7>..<new7>`.

- [ ] **Step 1: Tests.** lock: round trip; missing file; `PathFor` for a writable directory, a read-only directory, and a directory under `StoresDir`. source (temp git repo as the remote, `AllowLocal`): first `SkillDir` resolves branch `main` and returns `Commit`; second call with `pinned` and the remote repository deleted still returns the same directory; pinned with the cache removed and the remote moved forward exports the pinned commit; url skill through a TLS test server, and a changed body with a pin fails with the `--update` hint; `path` `../x` rejected. build: a v3 source with one `git` skill, one `url` skill, one local skill and one `git` prompt builds; `tbox-lock.json` appears next to the Tariboyfile with two git entries and one url entry; a second build with the remotes deleted is `Built: false`; a new commit on the remote without `--update` is still `Built: false`; with `Update: true` it rebuilds, `Moved` holds the tags, and the lock commit changed; an `only: tariboy` `git` entry is never resolved (remote deleted, build succeeds); nil `Resolver` with a `git` entry fails naming the entry. cli: `build --path X --update` prints the `updated` line.
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3:** `make tbox-check`. Commit `tbox: build images from git and https sources with a lock file`.

### Task 3: Schema v2 images with `skills-lock.json`

**Files:**
- Create: `tbox/internal/skillslock/skillslock.go`, `hash.go`, `skillslock_test.go`, `tbox/testdata/compat/skills-lock/…` (a small fixture and its `computedHash` produced by tariboy's `stores.SkillHash`)
- Modify: `tbox/internal/tfile` (a path remap hook for resolution and for `Load`'s prompt de-duplication), `tbox/internal/build/build.go`, `tbox/go.mod` (`golang.org/x/text`, same version as the root module)

**Interfaces:**
- Consumes: `source.Resolver`, `tfile`, `gitx`.
- Produces:
  - `skillslock.Filename = "skills-lock.json"`; `skillslock.Skill{Source, SourceType, SkillPath, ComputedHash string}`; `skillslock.Read(path string) (map[string]Skill, error)` — format of the file the skills installer writes (`{"version":1,"skills":{name:{source,sourceType,skillPath,computedHash}}}`).
  - `skillslock.Hash(dir string) (string, error)` — identical to `stores.SkillHash` in `internal/stores/skillslock.go` (file order, skipped directories, bytes hashed).
  - `skillslock.Restore(ctx, r *source.Resolver, lockDir string, skills map[string]Skill) (dirs map[string]string, warnings []string, err error)` — `github`: mirror `https://github.com/<source>.git`, default branch, export the directory of `skillPath`; `local`: `source` relative to `lockDir`, used in place; any other `sourceType` is an error naming it. A hash that differs from `computedHash` adds the warning `skill <name>: content differs from skills-lock.json` and is not an error.
  - `tfile.Remap func(abs string) string` and `tfile.LoadWith(dir string, remap Remap) (*File, error)`; `ResolveSkillDir` and `ResolvePromptFile` variants that take the remap. `Load(dir)` is `LoadWith(dir, nil)`.
  - `build.Build`, for a schema v2 source: reads `skills-lock.json` in the image directory and, when present, in the Store root (`build.Options.StoreRoot`); restores the listed skills and remaps `<dir>/.agents/skills/<name>[/...]` to the restored directory for both the image directory and the Store root, the image's own lock winning. Restored skills are pinned in `tbox-lock.json` (commit and tree hash) so later builds are offline and stable. `Result.Warnings []string` carries the hash warnings and, when the source declares plugins, `image declares plugins; skills that need the tariboy daemon will not work here`.
  - A v2 entry with `$PLUGINS/...` fails the build naming the entry (already an error from `tfile`; keep it and test it here).

- [ ] **Step 1: Tests.** `TestHashMatchesTariboy` against the committed fixture value (produce it once with a throwaway root-module test that calls `stores.SkillHash`; do not commit that test). Restore: a `github` entry served from a temp repo (resolver URL rewrite hook for tests: `source.Resolver.RewriteURL func(string) string`), a `local` entry, an unknown `sourceType` error, a hash mismatch warning. build: a v2 image directory with `Tariboyfile.yaml` pointing at `./.agents/skills/a` and `./.agents/skills/b` and a prompt `./.agents/skills/a/a.md`, with `skills-lock.json` (`a` github, `b` local `../../skills/b`), builds; the image directory and the Store root are byte-identical before and after (no `.agents` created); the archive holds both skills and the prompt layer; second build offline is `Built: false`; plugins in the source give the warning; a skill named in the Tariboyfile but in neither lock and not on disk fails naming it.
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3:** `make tbox-check`. Commit `tbox: restore skills-lock.json skills for schema v2 images`.

### Task 4: Stores registry, inventory and `tbox store`

**Files:**
- Create: `tbox/internal/stores/stores.go`, `inventory.go`, `stores_test.go`, `inventory_test.go`, `tbox/internal/cli/cmd_store.go`, `cmd_store_test.go`

**Interfaces:**
- Consumes: `gitx`, `fetch`, `tfile`, `skill`, `paths.Home`.
- Produces:
  - `stores.Kind` = `git` | `url` | `dir`; `stores.Store{Name string; Kind Kind; Source string; Ref string; Commit string}`.
  - `stores.Registry{Home paths.Home; Git gitx.Git; HTTP fetch.Client}`; registry file `<home>/stores.json` (`{"version":1,"stores":[...]}`, atomic writes, flock around read-modify-write).
  - `(Registry).Add(ctx, name, source, ref string) (Store, error)` — name rule `^[a-z0-9][a-z0-9._-]*$`; kind detection: an existing directory (relative paths resolved against the caller's cwd and stored absolute) is `dir`; a `https://` URL ending in `.tar.gz`, `.tgz` or `.zip` is `url`; otherwise git through `gitx.NormalizeURL`. `--ref` on a non-git Store is an error. Registers only after a successful clone, download or directory check. Duplicate name is an error.
  - `(Registry).List() ([]Store, error)`; `(Registry).Get(name string) (Store, error)` (`stores.ErrNotFound`); `(Registry).Refresh(ctx, name string) (Store, error)` (git: `Pull`; url: download into a new directory and swap; dir: nothing); `(Registry).SetRef(ctx, name, ref string) (Store, error)`; `(Registry).Remove(name string) error` (deletes the registration and only tbox's own clone or extracted archive; a `dir` Store's directory is never touched; built images stay).
  - `(Registry).Root(s Store) string` — `<home>/stores/<name>` for git and url, the stored path for dir.
  - `stores.Inventory{Images []Image; Skills []Skill; Prompts []Prompt}` with `Image{Name, Dir, Version string; Schema int; Err string}`, `Skill{Name, Dir, Description string; Err string}`, `Prompt{Name, Path string}`.
  - `(Registry).Inventory(s Store) (Inventory, error)` — images at `images/<name>/Tariboyfile.yaml` (parsed with `tfile.Parse`; a parse error fills `Err` and keeps the row); skills at `skills/<name>/SKILL.md`, and when the root has no `skills/` directory: `SKILL.md` at the root (name from frontmatter), `.agents/skills/<name>/`, `.claude/skills/<name>/`; prompts at `prompts/*.md` (name is the file name without `.md`). Symlinks are not followed. Names are sorted.
  - `stores.Selector{Store, Name string}`; `stores.ParseSelector(s string) (Selector, bool)` — true only for exactly one `/` with both sides non-empty.
  - Commands: `tbox store add NAME SOURCE [--ref REF]`, `tbox store list`, `tbox store show NAME` (source, ref, commit, then images, skills, prompts with per-row errors), `tbox store refresh NAME`, `tbox store ref NAME REF` (`""` returns to the default branch), `tbox store remove NAME`.

- [ ] **Step 1: Tests.** Registry with a temp git repo, a TLS test server archive and a directory: add each kind and `List` returns three; add with a bad name, a duplicate, `--ref` on a dir Store, a missing directory, an unreachable git URL (nothing registered afterwards); `Refresh` after a new commit moves `Commit`; `SetRef` to a tag, to `""`, to `nope` (error, registration unchanged); `Remove` of a dir Store leaves the directory, of a git Store removes `<home>/stores/<name>`; relative dir path stored absolute. Inventory: the standard layout; one broken Tariboyfile and one bad `SKILL.md` each give a row with `Err` beside good rows; a repo with only root `SKILL.md`; a repo with `.claude/skills/x`; missing `images/` gives an empty list; a symlinked `skills/evil` is ignored. `ParseSelector` table (`a/b` true; `a`, `a/b/c`, `/b`, `a/` false). cli: each command's output and exit code, including `store show` on an unknown name (exit 1).
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3:** `make tbox-check`. Commit `tbox: add Stores registry and inventory`.

### Task 5: Store-aware commands, end-to-end test, documentation

**Files:**
- Create: `tbox/internal/cli/cmd_library.go` (+ test), `tbox/internal/cli/e2e_stores_test.go`
- Modify: `tbox/internal/cli/cmd_build.go`, `cmd_images.go`, `cmd_run.go`, `docs/docs/binaries/tbox.mdx`, `docs/docs/images/index.mdx` (schema v3 section: remote entries and the lock file), `README.md` if its tbox paragraph names limits that no longer hold

**Interfaces:**
- Consumes: everything above.
- Produces:
  - `tbox build STORE/IMAGE [--name N] [--tag T]... [--update]` — source directory `<root>/images/<image>`, `StoreRoot` set, default name is the image name. `--path` and a selector are mutually exclusive.
  - `tbox run STORE/IMAGE HARNESS [-- ARGS...]` — build-if-changed, then run. A built image whose name contains no `/` keeps working as before.
  - `tbox images` — built images, then a second table `STORE/IMAGE  VERSION  STATUS` where status is `built` when a built image of that name has the same `image_version`, `not built`, or the row's error.
  - `tbox skills` (outside a session) — every Store skill as `<store>/<skill>\t<description>`; `tbox skill STORE/SKILL` — description, file list, `SKILL.md`; a bare name is an error listing the matching `<store>/<name>` candidates.
  - `tbox prompts` — `<store>/<prompt>`; `tbox prompt STORE/PROMPT` — the text.
  - Warnings from `build.Result.Warnings` go to stderr, one per line, prefixed `warning: `.

- [ ] **Step 1: Tests.** Command tests for each form above with a registry holding two Stores that both have skill `x` (Review Focus 5). `e2e_stores_test`: a temp git repository laid out as a Store with `images/demo/Tariboyfile.yaml` (schema v3) whose skills are one `dir: ../../skills/local-skill`, one `git:` entry pointing at a second temp repository, and one `only: tariboy` skill, plus `prompts/process.md`; `store add s <repo>`, `images` shows `s/demo not built`, `run s/demo <fake harness>` builds and execs the fake harness, the fake harness runs `tbox skills` and sees exactly the two packaged skills; a second `run` prints no build line; after a new commit in the Store and `store refresh s`, `run` rebuilds. A second case builds a schema v2 Store image that depends on `skills-lock.json` and prints the plugins warning.
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3: Docs.** Update `tbox.mdx`: Stores commands and kinds, the inventory layout and fallback skill locations, `<store>/<name>` addressing, schema v3 remote entries, `tbox-lock.json` and where it is written, `--update`, offline behaviour, `skills-lock.json` support for v2 images and its warning, security rules for URLs and archives; remove the stage 2 items from "known limits".
- [ ] **Step 4:** `make backend-check` and `make frontend-check` in the background; fix what they report. `git diff --check`.
- [ ] **Step 5: Manual smoke** with a scratch `TBOX_HOME`: `tbox store add tariboy https://github.com/alekzonder/tariboy-store.git`, `tbox store show tariboy`, `tbox build tariboy/developer`, `tbox image show developer`; record the warnings printed and how long the first and the second build took. This is the one step that uses the network.
- [ ] **Step 6:** Commit `tbox: address Store content from build, run and listing commands`.
