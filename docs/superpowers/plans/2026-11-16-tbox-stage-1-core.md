# tbox Stage 1 (Core) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A standalone `tbox` binary that builds an image from a local `Tariboyfile.yaml` (schema v2 or v3), stores it, and runs Claude Code, Codex or OpenCode with the image's prompt and skills; inside that session `tbox` shows only the image's skills.

**Architecture:** A separate Go module in `tbox/` with zero imports from the tariboy module. Small internal packages port the tariboy image format (skill validation, Tariboyfile parsing, archive and content digest) and add an image store, a run directory, harness adapters and an agent mode. Shared fixtures with a committed expected digest keep the two implementations of the format in step.

**Tech Stack:** Go 1.26, standard library, `gopkg.in/yaml.v3`. No CLI framework: `flag` plus a small dispatcher.

**Spec:** `docs/superpowers/specs/2026-11-16-tbox-design.md` (Russian). Read it fully before any task.

Stages 2 (Stores, remote sources, locks) and 3 (constructor, interactive mode) get their own plans after this stage lands; they build on the interfaces below.

## Global Constraints

- All work happens in the worktree `/home/agent/github/tariboy/.worktrees/tbox` on branch `dev/tbox`. Never edit the main checkout.
- Module path `github.com/alekzonder/tariboy/tbox`, directory `tbox/`, own `go.mod` (`go 1.26.3`). No file under `tbox/` imports `github.com/alekzonder/tariboy/` except its own `.../tariboy/tbox/...` packages.
- The tariboy packages are the format reference, to be ported, not imported: `internal/agentskills`, `internal/imagefile`, `internal/image` (`build_v2.go`, `content_digest.go`, `template.go`, `manifest.go`, `portable_validate.go`), `internal/refstore`, `internal/portablearchive`, `internal/imageportable`, `internal/rolefile`, `internal/safefs`. Where this plan says "same rules as X", read X and reproduce its limits, error conditions and byte-level output.
- Root is `$TBOX_HOME`, default `~/.tbox`. Tests always set `TBOX_HOME`, `CODEX_HOME`, `OPENCODE_CONFIG_DIR` and `HOME` to `t.TempDir()` values. No test touches `~/.tbox`, `~/.codex`, `~/.claude`, OpenCode config, `~/.tariboy` or a live daemon. No test needs a real harness binary: adapters take the executable lookup and version probe as injectable functions.
- `tbox` never writes into the user's working directory and never edits `$CODEX_HOME/config.toml`.
- `tbox` ignores `plugins` and every `runtime` prompt entry and substitutes nothing for them. The archive manifest carries `"plugins": []` and the template carries no runtime entries.
- Entries with `only: tariboy` never enter the archive or the fingerprint.
- The archive is tariboy schema v2 byte format and must pass tariboy's portable image validation.
- TDD: failing test first, then code. Commit after each task. Commit messages carry no attribution trailer of any kind.
- Functions stay under the repository complexity limits (`GOCYCLO_OVER`, `GOCOGNIT_OVER` in the root `Makefile`).
- Run Go commands for tbox from `tbox/` (`cd tbox && go test ./...`).

## Review Focus

1. A skill directory with a symlink, a hardlinked file, a special file, or a `SKILL.md` whose name differs from the directory: build fails with an error naming the skill, never packages it. (Task 2)
2. `TBOX_IMAGE_DIR` set but the directory is missing or has no `manifest.json`: every command fails closed with a clear error; no fallback to normal mode. (Task 7)
3. `tbox skill exec` with a script path that escapes the skill (`../other/x.sh`, absolute path, symlink out): refused before anything runs. (Task 7)
4. A harness binary that is missing or older than the minimum: `tbox run` exits non-zero with the harness name and the minimum version, and writes nothing into `$CODEX_HOME`. (Tasks 8, 9)
5. Two images whose names differ only in characters that are unsafe for a Codex profile file name or plugin name (`my.image`, `My Image`): names are rejected at build time by the same ref-name rule tariboy uses, so no profile file collision can occur. (Task 5)

---

### Task 1: Module scaffold, paths, CLI skeleton, build and checks

**Files:**
- Create: `tbox/go.mod`, `tbox/cmd/tbox/main.go`, `tbox/internal/version/version.go`, `tbox/internal/paths/paths.go`, `tbox/internal/paths/paths_test.go`, `tbox/internal/cli/cli.go`, `tbox/internal/cli/cli_test.go`, `tbox/internal/importguard/importguard_test.go`
- Modify: `Makefile` (build, install, check steps), `.github/workflows/pull-request.yml` only if a step list there names individual make targets

**Interfaces:**
- Produces:
  - `version.Version = "0.1.0"`
  - `paths.Home` (string type); `paths.Resolve(getenv func(string) string) (paths.Home, error)` — `$TBOX_HOME`, else `$HOME/.tbox`, error when neither is set; methods `ImagesDir()`, `RunDir(digest, harness string) string`, `RunRoot()`, `StoresDir()`, `CacheDir()`, `SourcesDir()`.
  - `cli.Env{Args []string; Getenv func(string) string; Stdin io.Reader; Stdout, Stderr io.Writer; Cwd string; Exec func(argv0 string, argv, env []string) error; LookPath func(string) (string, error); Now func() time.Time}`
  - `cli.Main(env cli.Env) int` — dispatches on the first argument through a table `map[string]func(*cli.Env, []string) int`; later tasks register commands in that table. Unknown command: exit 2 with usage on stderr.

- [ ] **Step 1: Tests.** `TestResolveHome` (TBOX_HOME wins; HOME fallback gives `<home>/.tbox`; both empty is an error). `TestVersionCommand`: `cli.Main` with args `version` and with `--version` prints `0.1.0\n`, exit 0. `TestUnknownCommand`: exit 2, stderr mentions the command. `TestNoTariboyImports` in `importguard`: walk `tbox/` with `go/parser` (imports only) and fail on any import path that starts with `github.com/alekzonder/tariboy/` and not with `github.com/alekzonder/tariboy/tbox/`.
- [ ] **Step 2:** Run `cd tbox && go test ./...`; expect compile failures.
- [ ] **Step 3:** Implement the packages and `main.go` (builds `cli.Env` from `os`, `syscall.Exec`, `exec.LookPath`).
- [ ] **Step 4: Makefile.** Add `tbox` to `build` (`cd tbox && $(GO) build -trimpath -o ../$(BINDIR)/tbox ./cmd/tbox`), to `BINARIES` and `MANAGED_LINKS`. Add targets `tbox-fmt-check`, `tbox-vet`, `tbox-staticcheck`, `tbox-complexity`, `tbox-test`, each running the same tool as its root counterpart inside `tbox/` (staticcheck, gocyclo and gocognit through the root `tools.mod`, same thresholds), and one `tbox-check` that runs them. Add `run_step "tbox-check"` to `backend-check` next to `test`. Confirm the root `go list ./...` does not descend into `tbox/` (nested module).
- [ ] **Step 5:** `cd tbox && go test ./... && cd .. && make build && ./bin/tbox version && make tbox-check`. Expect `0.1.0` and all green.
- [ ] **Step 6:** Commit `tbox: add module scaffold, paths and CLI skeleton`.

### Task 2: Skill package

**Files:**
- Create: `tbox/internal/skill/skill.go`, `frontmatter.go`, `skill_test.go`, `tbox/internal/fsafe/fsafe.go` (+ test) for no-follow reads and path confinement

**Interfaces:**
- Produces:
  - `skill.Metadata` with exactly the JSON fields and order of `agentskills.Metadata` (`name, description, source, category, client_version, archive_root, file_count, size, tree_sha256`).
  - `skill.File{RelativePath string; Size int64; SHA256 string; Executable bool; Body []byte}`
  - `skill.Prepared{Metadata Metadata; Files []File}`
  - `skill.Prepare(dir, source, category string) (Prepared, error)` — same rules as `agentskills.Prepare`: no symlinks, no hardlinks, no special files, root `evals/` excluded, limits 1024 files / 8 MiB per file / 32 MiB per skill, name pattern `^[a-z0-9]+(?:-[a-z0-9]+)*$` equal to the directory name, `ArchiveRoot = "skills/<name>"`, identical `tree_sha256`.
  - `skill.ValidateSet([]Prepared) error` — unique names.
  - `fsafe.ReadFileNoFollow(path string, max int64) ([]byte, error)`; `fsafe.Within(root, target string) (string, error)` — cleaned absolute target that must stay inside root after symlink evaluation.

- [ ] **Step 1: Tests.** Table test over temp skill trees: valid skill (assert `FileCount`, `Size`, executable bit, `ArchiveRoot`); name mismatch; bad name `Bad_Name`; missing frontmatter; symlink member; hardlink; `evals/` excluded from `Files`; file over 8 MiB; duplicate names in `ValidateSet`. `TestTreeHashMatchesTariboy`: build the fixture skill at `tbox/testdata/compat/skills/hello-skill/` (create it: `SKILL.md`, `scripts/run.sh` executable, `reference.md`) and compare `TreeSHA256` to the constant in `tbox/testdata/compat/skills/hello-skill.tree_sha256`. Produce that constant once by running the tariboy implementation in a throwaway test (`go test` in the root module calling `agentskills.Prepare` on the fixture) and commit the value; do not commit the throwaway test.
- [ ] **Step 2:** Run tests, expect failure.
- [ ] **Step 3:** Implement by porting `internal/agentskills/validate.go` and `frontmatter.go`.
- [ ] **Step 4:** Run tests, expect pass.
- [ ] **Step 5:** Commit `tbox: add skill validation`.

### Task 3: Tariboyfile package (v2 and v3, local entries, extends, only)

**Files:**
- Create: `tbox/internal/tfile/tfile.go`, `extends.go`, `resolve.go`, `version.go`, `tfile_test.go`, `extends_test.go`

**Interfaces:**
- Produces:
  - `tfile.Filename = "Tariboyfile.yaml"`
  - `tfile.Plugin{Name string}`
  - `tfile.SkillEntry{Dir, Git, Path, Ref, URL, Only string; BaseDir string}` — `BaseDir` is the directory of the Tariboyfile that declared the entry (not from YAML).
  - `tfile.PromptEntry{File, Git, Path, Ref, URL, Runtime, Only string; BaseDir string}`
  - `tfile.File{Dir string; SchemaVersion int; ImageVersion string; Extends []string; Plugins []Plugin; Skills []SkillEntry; Prompts []PromptEntry}`
  - `tfile.Parse(path string) (*File, error)` — strict: unknown fields and multiple documents rejected; `schema_version` 2 or 3; v2 accepts only `dir` / `file` / `runtime`; v3 accepts exactly one of `dir|git|url` per skill and one of `file|git|url|runtime` per prompt, `path` required with `git`, `only` empty or `tariboy`; limits and SemVer rule as `internal/imagefile/v2.go` and `version.go`.
  - `tfile.Load(dir string) (*File, error)` — parse plus `extends` flattening with the rules of `internal/imagefile/extends.go` (depth 16, cycle detection, plugins keep first, skills keep last by skill directory name, prompts de-duplicated). The result has no `Extends`; every entry keeps the `BaseDir` of its layer.
  - `(*File).ForTbox() *File` — copy without entries marked `only: tariboy` and without runtime prompts; `Plugins` emptied.
  - `tfile.ResolveSkillDir(e SkillEntry) (path, source, category string, err error)` and `tfile.ResolvePromptFile(e PromptEntry) (path, source, category string, err error)` for local entries: `./`, `../` (skills; prompts only below a declared skill directory, as `resolver.go:128-145`), absolute; symlinks rejected; prompt max 4 MiB; `source` is the value as written; `category` uses the same strings `resolver.go` emits. `$PLUGINS/...` returns an error that names the entry and says tbox has no plugin root. `git` and `url` entries return `tfile.ErrRemoteUnsupported` (stage 2 replaces this).
  - `tfile.RoleRead(dir string) (*Role, error)` with `Role` matching `rolefile.Role` JSON.

- [ ] **Step 1: Tests.** Parse: valid v2; valid v3 with `git`+`path`+`ref`, `url`, `dir`, `only`; rejects unknown key, `schema_version: 1` and `4`, v2 with `git`, v3 skill with both `dir` and `git`, `git` without `path`, `only: other`, 129 skills, duplicate runtime. Load: two-layer extends where the child overrides a skill of the same name and both declare plugin `mail` (one remains); cycle error; depth 17 error. ForTbox: drops `only: tariboy` skill and prompt, all runtime prompts, plugins. Resolve: `$PLUGINS/x` error text contains the entry value; prompt `../x.md` outside any skill rejected; symlinked prompt rejected.
- [ ] **Step 2:** Run, expect failure.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run, expect pass.
- [ ] **Step 5:** Commit `tbox: parse Tariboyfile schema v2 and v3`.

### Task 4: Archive package and format compatibility

**Files:**
- Create: `tbox/internal/archive/archive.go`, `digest.go`, `read.go`, `archive_test.go`, `tbox/testdata/compat/README.md`, `tbox/testdata/compat/v2-basic/{Tariboyfile.yaml,instructions.md,ROLE.md,skills/hello-skill/...}`, `tbox/testdata/compat/v2-basic.digest`, `tbox/testdata/compat/v3-marked/...`
- Create (root module): `internal/image/tbox_compat_test.go`

**Interfaces:**
- Consumes: `skill.Prepared`, `tfile.Role`.
- Produces:
  - `archive.Member{Name string; Mode int64; Body []byte}`
  - `archive.TemplateEntry`, `archive.Template`, `archive.Manifest` — JSON identical to `image.TemplateEntry`, `image.PromptTemplate` and the struct in `marshalV2Manifest` (`build_v2.go:315-327`), plus `Digest` and `Tag` fields that are never marshalled into the archive.
  - `archive.PromptLayer{Source, Category, Base string; Body []byte}` — `Base` is the source file's base name.
  - `archive.Image{Manifest Manifest; Template Template; Members []Member}`
  - `archive.Assemble(name, imageVersion string, builtAt time.Time, skills []skill.Prepared, prompts []PromptLayer, role *tfile.Role) (Image, error)` — member order, modes (`0600`, `0700` for executables), layer names `prompt/layers/%03d-<cleaned base>`, template hash, `plugins: []`, id = content digest, `built_at` format: all as `build_v2.go`. The index in the layer name counts only prompts that are packaged.
  - `(Image).Write(w io.Writer) error` — gzip tar, same header fields as `writeTarFileMode`.
  - `archive.Read(r io.Reader) (Image, error)` — strict: rules of `validateV2PortableMembers` and `validateV2ManifestFields` in `portable_validate.go` (unknown manifest fields rejected, unexpected members rejected, hashes and sizes checked, digest recomputed and compared to `id`). It accepts archives that contain runtime template entries and non-empty plugins (images exported from tariboy).
  - `archive.ContentDigest(members []Member) (string, error)` — exactly `content_digest.go`.
  - `(Image).Skill(name string) (skill.Metadata, []Member, bool)`.

- [ ] **Step 1: Tests in tbox.** `TestAssembleRoundTrip` (Assemble, Write, Read, equal manifest and members). `TestDigestIgnoresBuiltAt` (two build times, same digest). `TestReadRejects`: unknown manifest field, extra member `x.txt`, tampered layer body, wrong `id`. `TestCompatDigest`: load `testdata/compat/v2-basic` through `tfile.Load` + resolve + `skill.Prepare` + `Assemble` with name `compat-basic` and compare `Manifest.ID` to the content of `v2-basic.digest`.
- [ ] **Step 2: Test in the root module.** `internal/image/tbox_compat_test.go`: `TestTboxCompatDigest` builds `../../tbox/testdata/compat/v2-basic` with `imagefile.ParseV2` + `BuildV2` into a temp `Store` under name `compat-basic` and asserts the manifest digest equals `v2-basic.digest`. `TestTboxArchiveImports`: validates the committed archive `../../tbox/testdata/compat/v3-marked.tar.gz` with the same portable validation `imageportable` uses for import (contract enforced). Generate `v2-basic.digest` from this tariboy test once and commit it.
- [ ] **Step 3:** Run both; expect failure.
- [ ] **Step 4:** Implement `archive`. Add a `go test` flag `-update-compat` in the tbox test that regenerates `v3-marked.tar.gz` from `testdata/compat/v3-marked` (a v3 source with plugins, runtime entries and `only: tariboy` entries); commit the generated archive. A second tbox test asserts the committed archive equals a fresh build byte for byte except `built_at` (compare digests).
- [ ] **Step 5:** `cd tbox && go test ./... && cd .. && go test ./internal/image -run TestTbox`. Expect pass.
- [ ] **Step 6:** Commit `tbox: write and read image archives compatible with tariboy`.

### Task 5: Image store, build with change detection, build commands

**Files:**
- Create: `tbox/internal/imgstore/imgstore.go`, `ref.go`, `imgstore_test.go`, `tbox/internal/build/build.go`, `fingerprint.go`, `build_test.go`, `tbox/internal/cli/cmd_build.go`, `cmd_images.go`, `cmd_build_test.go`

**Interfaces:**
- Consumes: `tfile`, `skill`, `archive`, `paths.Home`, `cli.Env`.
- Produces:
  - `imgstore.Ref{Name, Tag string}`; `imgstore.ParseRef(s string) (Ref, error)` — `name[:tag]`, tag defaults to `latest`; name and tag rules as `internal/image/ref.go`.
  - `imgstore.Store{Dir string}` with layout `<Dir>/<name>/refs/<digest>.tar.gz` and `<Dir>/<name>/tags/<tag>` (file holding the digest), atomic writes as `refstore`.
  - `(Store).Publish(name string, img archive.Image, tags []string) (moved []string, err error)` — writes the ref if absent, points each tag at it, returns tags that previously pointed at another digest.
  - `(Store).Resolve(ref Ref) (digest, archivePath string, err error)`; `(Store).Open(ref Ref) (archive.Image, error)`; `(Store).List() ([]Entry, error)` with `Entry{Name, Tag, Digest, ImageVersion, BuiltAt string}`; `(Store).Remove(ref Ref) error` (drops the tag, and the ref file when no tag points at it); `(Store).ImportArchive(name string, r io.Reader, tags []string) (archive.Image, error)`.
  - `(Store).Fingerprint(name string) (fingerprint, digest string, ok bool)` and `(Store).SetFingerprint(name, fingerprint, digest string) error` — stored in `<Dir>/<name>/source.json`, outside the archive.
  - `build.Options{SourceDir, Name string; Tags []string; Store imgstore.Store; Now func() time.Time}`
  - `build.Result{Name, Digest, ImageVersion string; Tags []string; Built bool; Moved []string}`
  - `build.Build(opts Options) (Result, error)` — default name is the source directory's base name; default tags are `image_version` (when set) and `latest`. Steps: `tfile.Load`, `ForTbox`, fingerprint; when the fingerprint equals the stored one and the stored digest still resolves, return `Built: false`; otherwise prepare skills and prompts, `archive.Assemble`, `Publish`, `SetFingerprint`.
  - `build.Fingerprint(f *tfile.File) (string, error)` — SHA-256 over a canonical text: every Tariboyfile in the extends chain (path-independent: content only), then for each packaged local skill file and prompt file its entry value, relative path, mode bit and content hash, then `ROLE.md`. A per-file cache keyed by absolute path, size and mtime (`<Dir>/<name>/stat-cache.json`) lets an unchanged file skip rehashing.
  - Commands: `tbox build --path DIR [--name N] [--tag T]...` (prints `built <name>:<tag> <digest>` or `up to date <name> <digest>`, and one line `moved <tag> to new content` per moved tag); `tbox images` (table: name, tag, version, digest prefix, built); `tbox image show REF` (manifest fields, skills with descriptions, prompt layer sources); `tbox image rm REF`.

- [ ] **Step 1: Tests.** imgstore: publish then resolve by tag and by `latest`; second publish of other content under the same tag reports the tag in `moved` and keeps the old ref file; remove keeps a ref still tagged; `ParseRef` rejects `My Image`, `a/b`, empty, `x:`. build: first build `Built: true`; second build unchanged `Built: false` and no new file in `refs/`; edit a skill file with unchanged `image_version` gives `Built: true`, new digest, `Moved` holds `latest` and the version tag; changing only an `only: tariboy` skill gives `Built: false`; touching mtime without content change gives `Built: false`; a source with `runtime:` and `plugins:` builds and the archive has neither. `TestBuildUnchangedIsFast`: an image with 200 skill files, second build under 200 ms. cli: `build --path` output lines; `images` lists it; `image show` prints skill names; `image rm` then `images` is empty.
- [ ] **Step 2:** Run, expect failure.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run, expect pass.
- [ ] **Step 5:** Commit `tbox: add image store and build with change detection`.

### Task 6: Portable export and import

**Files:**
- Create: `tbox/internal/portable/portable.go`, `portable_test.go`, `tbox/internal/cli/cmd_transfer.go` (+ test), `tbox/testdata/compat/v3-marked.tariboy-image.tar.gz`
- Modify (root module): `internal/image/tbox_compat_test.go` or a new `internal/imageportable/tbox_compat_test.go`

**Interfaces:**
- Consumes: `archive.Image`, `imgstore`.
- Produces:
  - `portable.Export(w io.Writer, img archive.Image, ref imgstore.Ref, producerVersion string) error` — the `tariboy-portable` envelope exactly as `imageportable.Service.Export` writes it (`kind: image-artifact`, single file `image.tar.gz`, metadata `ref`, `digest`, `producer_version`, `plugins` as a non-nil array).
  - `portable.Import(r io.Reader) (archive.Image, imgstore.Ref, error)` — validates the envelope with the rules of `internal/portablearchive` and the inner archive with `archive.Read`.
  - Commands: `tbox image export REF [-o FILE]` (default `<name>-<tag>.tariboy-image.tar.gz` in the current directory; this is the one command that writes there, at the user's explicit request) and `tbox image import FILE`.

- [ ] **Step 1: Tests.** tbox: export then import returns the same digest and ref; import rejects a wrong `format`, a second file in the envelope, a digest in metadata that differs from the inner archive. Root module: `TestTboxPortableImports` feeds the committed `v3-marked.tariboy-image.tar.gz` to `imageportable.Service` preview and apply against a temp base directory and expects success and the ref `compat-marked:latest`. Regenerate the committed file with the `-update-compat` flag from Task 4.
- [ ] **Step 2:** Run, expect failure.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run both modules' tests, expect pass.
- [ ] **Step 5:** Commit `tbox: export and import images in the tariboy portable format`.

### Task 7: Run directory and agent mode

**Files:**
- Create: `tbox/internal/rundir/rundir.go`, `rundir_test.go`, `tbox/internal/agentmode/agentmode.go`, `agentmode_test.go`
- Modify: `tbox/internal/cli/cli.go` (agent-mode gate)

**Interfaces:**
- Consumes: `archive.Image`, `paths.Home`, `fsafe`.
- Produces:
  - `rundir.Run{Dir, ImageDir, SkillsDir, PromptPath string; Name, Tag, Digest, ImageVersion string; Skills []skill.Metadata}`
  - `rundir.Prepare(home paths.Home, img archive.Image, ref imgstore.Ref, harness string) (Run, error)` — unpacks every member into `<home>/run/<digest>/<harness>/image/` (modes preserved, staged in a temp sibling and renamed, reused when already complete), and writes `PROMPT.md` next to `image/`: the bodies of the template's `file` entries in order, each followed by a blank line; runtime entries skipped; nothing added. `SkillsDir` is `image/skills`.
  - `rundir.Open(imageDir string) (Run, error)` — reads `manifest.json` from an unpacked image; error when missing or invalid.
  - `agentmode.EnvImageDir = "TBOX_IMAGE_DIR"`, `agentmode.EnvImage = "TBOX_IMAGE"`, `agentmode.EnvSkillDir = "TBOX_SKILL_DIR"`.
  - `agentmode.Run(env *cli.Env, imageDir string, args []string) int` implementing `skills`, `skill NAME`, `skill exec NAME SCRIPT [ARGS...]`.
- Behaviour:
  - When `TBOX_IMAGE_DIR` is non-empty, `cli.Main` routes to `agentmode.Run`. Commands other than `skills`, `skill`, `version`, `help` exit 2 with `tbox: <command> is not available inside an image session (TBOX_IMAGE=<ref>)`.
  - `skills` prints one line per skill: `<name>\t<description>`. `skill NAME` prints name, description, the file list, a blank line, then `SKILL.md`. Unknown skill: exit 1 and the list of available names.
  - `skill exec` resolves the script with `fsafe.Within(skillDir, script)`, requires a regular file, then replaces the process through `env.Exec` with `TBOX_SKILL_DIR` added; a file without the executable bit is run through `/bin/sh` only when it ends in `.sh`, otherwise it is an error. Working directory is unchanged.

- [ ] **Step 1: Tests.** rundir: unpack gives executable `scripts/run.sh` with mode `0700`; `PROMPT.md` equals the two fixture prompts joined in order with no extra text; an image whose template has a runtime entry between them yields the same text; second `Prepare` does not rewrite files (compare mtimes). agentmode: `skills` output; `skill hello-skill` contains the `SKILL.md` body; `skill missing` exit 1; `skill exec hello-skill scripts/run.sh a b` calls `Exec` with the absolute script path, args `a b`, and `TBOX_SKILL_DIR`; `skill exec hello-skill ../other/x.sh`, an absolute path, and a symlink pointing out are refused without calling `Exec`; `TBOX_IMAGE_DIR` pointing at a missing directory makes `skills` exit 1 with an error and `build` exit 2; `build` inside a valid session exits 2 with the message above.
- [ ] **Step 2:** Run, expect failure.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run, expect pass.
- [ ] **Step 5:** Commit `tbox: add run directory and in-session skill commands`.

### Task 8: Harness adapters for Claude Code and OpenCode

**Files:**
- Create: `tbox/internal/harness/harness.go`, `claude.go`, `opencode.go`, `fallback.go`, `harness_test.go`, `claude_test.go`, `opencode_test.go`

**Interfaces:**
- Consumes: `rundir.Run`.
- Produces:
  - `harness.Host{Getenv func(string) string; LookPath func(string) (string, error); Output func(ctx context.Context, name string, args ...string) ([]byte, error)}`
  - `harness.Launch{Path string; Argv []string; Env []string; Notes []string}` — `Env` holds additions as `KEY=VALUE`; `Notes` are lines the CLI prints to stderr before launch (for example the fallback notice).
  - `harness.Adapter` interface: `Name() string`; `Prepare(ctx context.Context, host Host, run rundir.Run, extra []string) (Launch, error)`; `Clean(host Host, image string) error`.
  - `harness.Get(name string) (Adapter, error)`; `harness.Names() []string` returning `claude`, `codex`, `opencode` in that order; `harness.Installed(host Host) []string`.
  - `harness.Catalog(run rundir.Run) string` — the fallback text: a heading, one `- <name>: <description>` line per skill, and the sentence telling the agent to read a skill with `tbox skill <name>` and run its scripts with `tbox skill exec <name> <script>`.
  - `harness.ErrUnsupportedVersion{Harness, Found, Minimum string}`.
- Behaviour:
  - Claude: minimum version `2.1.227` (probe `claude --version`). Writes `<run.Dir>/plugin/.claude-plugin/plugin.json` (`name` = image name, `version` = image version or `0.0.0`) and a `skills` directory that holds the image's skills (copy; the bridge must not contain symlinks). Argv: `claude --plugin-dir <plugin> --append-system-prompt-file <PROMPT.md> <extra...>`. Before coding, verify on the installed Claude Code that `--append-system-prompt-file` is accepted in an interactive start (`claude --help` lists it); if it is print-mode only, pass the text with `--append-system-prompt` and record the finding in a code comment. An empty prompt omits the flag.
  - OpenCode: probe `opencode debug config` with `OPENCODE_CONFIG_DIR=<run.Dir>/opencode` and require the skills path in its output; otherwise `ErrUnsupportedVersion`. Writes `<run.Dir>/opencode/opencode.json` with `skills.paths: [<SkillsDir>]` and `instructions: [<PROMPT.md>]`. Env: `OPENCODE_CONFIG_DIR`. Before coding, verify on the installed OpenCode, with a throwaway config directory under a temp dir, that (a) the user's global config and credentials still apply when `OPENCODE_CONFIG_DIR` is set and (b) `instructions` accepts an absolute path. If (a) fails, stop and report: the adapter design must change. If (b) fails, use the fallback catalog route for the prompt and say so in `Notes`.
  - Every `Launch.Env` also carries `TBOX_IMAGE_DIR=<run.ImageDir>` and `TBOX_IMAGE=<name>:<tag>`.

- [ ] **Step 1: Probe** the two installed harnesses as described and write the results as comments at the top of `claude.go` and `opencode.go` (version probed, command, observed result).
- [ ] **Step 2: Tests** with a fake `Host`: Claude argv and plugin files for a two-skill image; Claude version `2.1.100` returns `ErrUnsupportedVersion` with minimum `2.1.227` and writes nothing; missing binary error names `claude`; extra args appended last; OpenCode `opencode.json` content and env; OpenCode probe output without the path returns `ErrUnsupportedVersion`; `Catalog` text for two skills; `Installed` with only `codex` on the fake path returns `[codex]`.
- [ ] **Step 3:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 4:** Commit `tbox: add Claude Code and OpenCode adapters`.

### Task 9: Codex adapter and clean

**Files:**
- Create: `tbox/internal/harness/codex.go`, `codex_test.go`, `tbox/internal/cli/cmd_clean.go` (+ test)

**Interfaces:**
- Consumes: Task 8 types.
- Produces: the `codex` adapter; `harness.CleanAll(host Host, home paths.Home, image string) error`; command `tbox clean [IMAGE]`.
- Behaviour:
  - `$CODEX_HOME` defaults to `$HOME/.codex`.
  - Plugin: `$CODEX_HOME/plugins/cache/tbox/<image>/<version>/` with `.codex-plugin/plugin.json` (`name`, `version`, `description`, `"skills": "./skills/"`) and a copy of the skills. `<version>` is the image version, or the first 12 digest characters when the image has none. The cache directory is rewritten when its recorded digest (file `.tbox-digest` inside it) differs from the image digest.
  - Profile: `$CODEX_HOME/tbox-<image>.config.toml` containing `[plugins."<image>@tbox"] enabled = true`, the marketplace table when the probe shows it is required, and the instructions key. Launch argv: `codex --profile tbox-<image> <extra...>`.
  - Marketplace root, when required: `<home>/run/codex-marketplace/` with `.agents/plugins/marketplace.json` listing the image plugins that tbox has prepared; it lives under `$TBOX_HOME`.
  - Verified on Codex 0.166.0 (see the spec): plugin in the cache plus `[marketplaces.tbox]` and `[plugins."demo@tbox"]` in a profile file exposes the skill; `-c` overrides do not; a profile outside `$CODEX_HOME` is rejected.
  - Before coding, probe with a temp `CODEX_HOME` and `codex debug prompt-input`: (a) is the `[marketplaces.tbox]` table needed, or do the cache and `[plugins]` suffice; (b) which profile key injects instructions (`developer_instructions`, `model_instructions_file`, or another) so that the prompt text appears in the model input. If no key works, do not write an `AGENTS.md` into the working directory; pass the prompt as the initial user message argument instead and add a `Note`. Record the findings as comments at the top of `codex.go`.
  - Minimum version: the lowest release the probe was run on (`0.166.0`); older gives `ErrUnsupportedVersion`.
  - `Clean` removes `plugins/cache/tbox/<image>` and `tbox-<image>.config.toml`; with an empty image it removes `plugins/cache/tbox` and every `tbox-*.config.toml`. It never touches `config.toml` or any other file. `tbox clean [IMAGE]` calls `Clean` on every adapter and removes `<home>/run` (or the run directories of that image's digests).

- [ ] **Step 1: Probe** as described; record findings.
- [ ] **Step 2: Tests** with a temp `CODEX_HOME` and fake host: files written at the exact paths above; `config.toml` placed beforehand is byte-identical afterwards; argv is `codex --profile tbox-<image> ...`; version `0.100.0` returns `ErrUnsupportedVersion` and leaves `CODEX_HOME` empty; second `Prepare` with the same digest rewrites nothing, with a new digest replaces the cache; `Clean("img")` leaves another image's files; `Clean("")` removes all tbox files and keeps `config.toml` and an unrelated `work.config.toml`. One opt-in test (`-run TestCodexLive`, skipped unless `TBOX_LIVE_CODEX=1`) runs the real `codex debug prompt-input` against a temp `CODEX_HOME` and asserts the skill name appears.
- [ ] **Step 3:** Run, expect failure. Implement. Run, expect pass. Run the live test once by hand with `TBOX_LIVE_CODEX=1` and note the result in the commit body.
- [ ] **Step 4:** Commit `tbox: add Codex adapter and clean command`.

### Task 10: `tbox run`, end-to-end test, documentation

**Files:**
- Create: `tbox/internal/cli/cmd_run.go`, `cmd_run_test.go`, `tbox/internal/cli/e2e_test.go`, `docs/docs/binaries/tbox.mdx`
- Modify: `docs/docs/binaries/index.mdx`, `README.md`, `docs/docs/images/index.mdx` (short section: schema v3 exists, tbox builds it, tariboy does not yet), the docs navigation file that lists the binaries pages

**Interfaces:**
- Consumes: everything above.
- Produces: `tbox run IMAGE HARNESS [-- ARGS...]`.
- Behaviour:
  - `IMAGE` is a built ref `name[:tag]`, or `--path DIR` replaces it to build-if-changed from a source directory first (`tbox run --path ./my-image claude`). Store selectors (`store/image`) arrive in stage 2; until then a name with `/` is an error that says so.
  - Unknown harness: exit 2 listing `harness.Names()`. Missing image: exit 1 listing built images.
  - Sequence: resolve, `rundir.Prepare`, `adapter.Prepare`, print `Notes` to stderr, then `env.Exec` with the process environment plus `Launch.Env`. `tbox` does not stay in the process tree.
  - Without both arguments and with a terminal on stdin, print that interactive selection arrives in stage 3 and exit 2 (replaced in stage 3).

- [ ] **Step 1: Tests.** `cmd_run_test`: with a fake `LookPath`/`Output`/`Exec`, `run img claude -- --model x` calls `Exec` with argv ending in `--model x` and env containing `TBOX_IMAGE_DIR` and `TBOX_IMAGE=img:latest`; unknown harness exit 2; missing image exit 1; `--path` builds once and a second run reports no rebuild. `e2e_test`: build the `v2-basic` fixture, run with a fake harness that is a shell script written to a temp dir and put on the fake path; the script calls the real test binary re-executed as `tbox` (`TestMain` dispatches to `cli.Main` when `TBOX_E2E_CHILD=1`) with `skills` and `skill exec hello-skill scripts/run.sh ok`; assert the output lists exactly the image's skills and the script's output.
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3: Docs.** `tbox.mdx`: what tbox is, install (`make build`, `bin/tbox`), `$TBOX_HOME` layout, build and change detection, run per harness including exactly what is written into `$CODEX_HOME` and how `tbox clean` removes it, in-session commands, what is ignored (plugins, runtime entries, `only: tariboy`), export/import with tariboy, known limits (global user skills stay visible; stage 2 and 3 features not yet present). Follow the writing rules already used in `docs/docs/binaries/*.mdx`.
- [ ] **Step 4:** `make backend-check` and `make frontend-check` (run in the background; they are slow). Fix what they report. `git diff --check`.
- [ ] **Step 5: Manual smoke** in a scratch `TBOX_HOME` and a temp working directory: build the fixture, `tbox run compat-basic claude -- --version`-style dry starts for each installed harness that can exit immediately, and for Codex, with `CODEX_HOME` set to a temp directory that holds a copy of a small `config.toml`, confirm with `codex --profile tbox-compat-basic debug prompt-input hi` that the skill and the prompt text are present. Run `tbox clean` and confirm that `config.toml` is byte-identical and no tbox file remains. Do not run the smoke against the real `~/.codex`. Record the commands and results in the commit body.
- [ ] **Step 6:** Commit `tbox: add run command and documentation`.
