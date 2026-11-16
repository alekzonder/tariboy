# tbox Stage 3 (Constructor and Interactive Mode) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `tbox image new` assembles an image source from Store skills and prompts and builds it; `tbox run` without arguments lets the user pick an image and a harness from lists.

**Architecture:** A small `tui` package draws arrow-key lists on a terminal behind an interface that tests fake. A `construct` package turns a selection into a schema v3 source directory under `$TBOX_HOME/sources/<name>/`, writing each skill and prompt as a full address so the source does not depend on Store names. The CLI offers every interactive action as flags too.

**Tech Stack:** Go 1.26, standard library, `gopkg.in/yaml.v3`, `golang.org/x/term` (raw mode and terminal size only).

**Spec:** `docs/superpowers/specs/2026-11-16-tbox-design.md` (section «Конструктор и интерактивный режим»). Earlier plans: `2026-11-16-tbox-stage-1-core.md`, `2026-11-16-tbox-stage-2-sources.md`; their Global Constraints apply unchanged.

## Global Constraints

- Everything in the stage 1 and stage 2 Global Constraints, plus the following.
- Every interactive command has a flag form that needs no terminal. Without a terminal on stdin and without the flags that make the command complete, it exits 2 with a one-line hint naming the flags; it never blocks waiting for input.
- No test needs a real terminal: `tui` functions take a `tui.Terminal` interface.
- The constructor writes only under `$TBOX_HOME/sources/<name>/`. It never writes into a Store.
- A source written by the constructor contains no Store name: skills and prompts are addressed by `git` + `path` + `ref`, by absolute `dir` / `file`, or by a copy inside the source directory.
- Cancelling a list (Esc, `q`, Ctrl-C) exits 130, writes nothing, and restores the terminal state.

## Review Focus

1. The terminal is left in raw mode after an error or a panic inside a list: the cursor, echo and line mode are restored on every exit path. (Task 1)
2. A list longer than the terminal height, or a terminal 20 columns wide, or item text with tabs, newlines or escape sequences from a skill description: the list scrolls, lines are truncated, control characters are shown as spaces. (Task 1)
3. `tbox image new NAME` when `sources/NAME` already exists: error naming the directory unless `--force`; with `--force` the old directory is replaced as a whole, not merged. (Task 2)
4. A selected skill whose Store is a git Store with local, uncommitted edits in tbox's clone cannot happen (tbox never edits its clones), but a `dir` Store that is itself a git checkout is addressed as an absolute `dir`, not as git. (Task 2)
5. No Stores registered, or no harness installed: the interactive commands say what is missing and how to add it, and exit 1 without drawing an empty list. (Task 3)

---

### Task 1: Terminal lists (`tui`)

**Files:**
- Create: `tbox/internal/tui/tui.go`, `term.go`, `render.go`, `tui_test.go`
- Modify: `tbox/go.mod` (`golang.org/x/term`, a version compatible with the root module's `golang.org/x/sys`)

**Interfaces:**
- Produces:
  - `tui.Key` (struct `{Rune rune; Code KeyCode}`) with codes `KeyUp, KeyDown, KeyEnter, KeySpace, KeyEsc, KeyCtrlC, KeyBackspace, KeyPageUp, KeyPageDown, KeyRune`.
  - `tui.Terminal` interface: `ReadKey() (Key, error)`; `Write(p []byte) (int, error)`; `Size() (cols, rows int)`; `Close() error`.
  - `tui.Open(in, out *os.File) (Terminal, error)` — raw mode through `x/term`; `tui.ErrNoTerminal` when `in` is not a terminal; `Close` restores the state and is safe to call twice.
  - `tui.Item{Label, Detail string}`.
  - `tui.Select(t Terminal, title string, items []Item) (index int, err error)`.
  - `tui.MultiSelect(t Terminal, title string, items []Item, checked []bool) (chosen []int, err error)`.
  - `tui.Input(t Terminal, prompt, initial string) (string, error)`.
  - `tui.ErrCancelled`.
- Behaviour: Up/Down and `k`/`j` move (when the filter is empty), PageUp/PageDown move a page, Enter confirms, Space toggles in `MultiSelect`, Esc or Ctrl-C cancels; `q` cancels when the filter is empty. Typing any other printable rune appends to a filter shown under the title; Backspace removes; items are matched by case-insensitive substring over `Label` and `Detail`; returned indices refer to the original slice. The view shows at most `rows-4` items and scrolls with the cursor. Each line is `Label` and, when it fits, two spaces and `Detail`, truncated to `cols-4` with `…`; tabs, newlines and other control characters in the text become spaces. An empty `items` slice returns an error without drawing. On return the list area is cleared and one summary line (`title: chosen labels`) is left.

- [ ] **Step 1: Tests** with a fake `Terminal` that replays keys and records writes: `Select` Down, Down, Enter returns 2; Esc returns `ErrCancelled`; filter `ab` then Enter returns the first match's original index; `MultiSelect` Space, Down, Space, Enter returns `[0,1]`; preset `checked` respected; 100 items on a 10-row terminal: moving to the last item never writes more than 6 item lines per frame; a 20-column terminal truncates with `…`; an item `"a\tb\x1b[31m\nc"` is written without tab, newline or ESC bytes; `Input` typing `abc`, Backspace, Enter returns `ab`; empty `items` is an error; after an error from `ReadKey` the function returns it and the fake's `Close` count is unchanged (the caller owns `Close`).
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3:** `make tbox-check`. Commit `tbox: add terminal list selection`.

### Task 2: Constructor and `tbox image new` (flag form)

**Files:**
- Create: `tbox/internal/construct/construct.go`, `construct_test.go`, `tbox/internal/tfile/marshal.go`, `marshal_test.go`, `tbox/internal/cli/cmd_new.go`, `cmd_new_test.go`

**Interfaces:**
- Consumes: `stores.Registry`, `stores.Inventory`, `stores.Selector`, `tfile`, `build`, `paths.Home`.
- Produces:
  - `tfile.Marshal(f *File) ([]byte, error)` — YAML with keys in the order `schema_version, image_version, extends, plugins, skills, prompts`; each entry writes only its non-empty fields in the order `dir|file|git|url|runtime, path, ref, only`; `tfile.Parse` of the output equals the input.
  - `construct.Spec{Name, Version string; Skills, Prompts []stores.Selector; NewPrompt bool; Force bool}` — `Version` defaults to `0.1.0`.
  - `construct.Create(reg stores.Registry, home paths.Home, spec Spec) (dir string, err error)` — writes `<home>/sources/<name>/Tariboyfile.yaml` (schema v3), then prompts in the order given, then `instructions.md` last in the prompt list when `NewPrompt` is set or when no prompt was selected. `instructions.md` starts with `# <name>` and one line telling the author to describe the agent's process here.
  - Addressing rules, by the kind of the Store that holds the item:
    - git Store: skill `{git: <store source URL>, path: <skill dir relative to the Store root>, ref: <store commit>}`; prompt `{git, path, ref}` likewise.
    - dir Store: skill `{dir: <absolute skill dir>}`; prompt `{file: <absolute path>}`.
    - url (archive) Store: the skill directory is copied to `<dir>/skills/<skill>` and written as `{dir: ./skills/<skill>}`; the prompt is copied to `<dir>/prompts/<prompt>.md` and written as `{file: ./prompts/<prompt>.md}`.
  - Errors: unknown Store, unknown skill or prompt (the message lists what the Store has), an inventory row with `Err`, two selected skills with the same skill name (names both selectors), an existing source directory without `Force`, an invalid image name (`imgstore.CheckName`).
  - Written atomically: staged in a sibling temp directory and renamed; with `Force` the old directory is replaced as a whole.
  - Command `tbox image new NAME [--skill STORE/SKILL]... [--prompt STORE/PROMPT]... [--new-prompt] [--version V] [--force] [--no-build]` — creates the source, then builds it through the same path as `tbox build --path` (with the Store-aware resolver) unless `--no-build`; prints `created <dir>` and the build line. `tbox images` gains nothing new: the built image appears as any other.

- [ ] **Step 1: Tests.** Marshal: round trip for a file with every entry kind and `only`; key order asserted on the text. Create with a registry of three temp Stores (git, dir, archive served by a TLS test server): the written Tariboyfile parses, holds no Store name, the git skill has the Store's commit as `ref`, the dir skill is absolute, the archive skill was copied and is relative; prompts likewise; no prompt selected gives an `instructions.md` entry; `NewPrompt` with selected prompts puts `instructions.md` last; duplicate skill name error; existing directory error, and `Force` replaces it (a stray file in the old directory is gone); unknown skill error lists the Store's skills. A `dir` Store that is a git checkout is still addressed by absolute `dir`. cli: `image new demo --skill s/a --prompt s/p` prints `created` and `built demo:0.1.0`, and `tbox image show demo` lists skill `a`; `--no-build` builds nothing; the source builds again unchanged (`up to date`) after the Store is removed from the registry (the source does not depend on it; for the git case the cache serves it).
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3:** `make tbox-check`. Commit `tbox: add image constructor`.

### Task 3: Interactive `image new` and `run`, documentation

**Files:**
- Create: `tbox/internal/cli/interactive.go`, `interactive_test.go`
- Modify: `tbox/internal/cli/cli.go` (`Env.OpenTerminal func() (tui.Terminal, error)`), `tbox/cmd/tbox/main.go`, `tbox/internal/cli/cmd_new.go`, `cmd_run.go`, `docs/docs/binaries/tbox.mdx`, `README.md` if it lists limits that no longer hold

**Interfaces:**
- Consumes: `tui`, `construct`, `stores`, `harness.Installed`, `imgstore`.
- Behaviour:
  - `tbox image new` with no `--skill`, `--prompt` or `--new-prompt`: opens the terminal; asks the name with `tui.Input` when `NAME` is absent; `MultiSelect` over every Store skill (`Label` `<store>/<skill>`, `Detail` the description); `MultiSelect` over every Store prompt plus a first item `new empty prompt (instructions.md)`; then proceeds as the flag form. With flags present it never opens the terminal.
  - `tbox run` with fewer than two positional arguments: opens the terminal; when `IMAGE` is absent, `Select` over built images (`<name>:<tag>`, detail: version and short digest) followed by Store images (`<store>/<image>`, detail: version and `built` / `not built`), rows with errors left out; when `HARNESS` is absent, `Select` over `harness.Installed` (skipped when exactly one is installed, with a line saying which was chosen); then runs as usual. Arguments after `--` are passed through.
  - No terminal: exit 2 with `tbox run: IMAGE and HARNESS are required when stdin is not a terminal` (and the matching text for `image new`, naming `--skill`, `--prompt`, `--new-prompt`).
  - Nothing to choose from: no Store skills or prompts for `image new` → exit 1 with `no skills or prompts found; add a Store with: tbox store add NAME SOURCE`; no images for `run` → exit 1 naming `tbox build` and `tbox store add`; no harness installed → exit 1 listing `harness.Names()`.
  - Cancel: exit 130, nothing written, terminal closed.
  - The terminal is closed before the build output is printed and before `Exec`.

- [ ] **Step 1: Tests** with `Env.OpenTerminal` returning a fake terminal: `image new` flow (type name, pick two skills, pick `new empty prompt`, Enter) creates the source with those skills and `instructions.md` and builds; cancel at the skills list exits 130 and `sources/` is empty; `run` flow picks the second image and the only installed harness is taken without a list, `Exec` is called with that image's `TBOX_IMAGE`; `run img` (one argument) only asks for the harness; `OpenTerminal` returning `tui.ErrNoTerminal` gives exit 2 and the hint for both commands; no Stores, no images and no harness cases give exit 1 with the texts above; the fake terminal's `Close` was called exactly once in every case, including the cancel and the error cases.
- [ ] **Step 2:** Run, expect failure. Implement. Run, expect pass.
- [ ] **Step 3: Docs.** Update `tbox.mdx`: the constructor (flag form and interactive form, where sources are written, how entries are addressed and why the source does not depend on Store names, editing and rebuilding a source with `tbox build --path`), interactive `tbox run`, keys of the lists, behaviour without a terminal; remove the stage 3 items from "known limits". Check every command in the page against the code.
- [ ] **Step 4:** `make backend-check` and `make frontend-check` in the background; fix what they report. `git diff --check`.
- [ ] **Step 5: Manual smoke** in a scratch `TBOX_HOME` with `script -qc` or an equivalent pseudo-terminal: run `tbox image new` and `tbox run` once each through the lists (a fake harness script on `PATH` is enough) and confirm the terminal state is restored (`stty -a` before and after). Record the commands in the commit body.
- [ ] **Step 6:** Commit `tbox: add interactive image constructor and run selection`.
