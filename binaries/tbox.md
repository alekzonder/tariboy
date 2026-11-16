---
title: tbox
description: Run Tariboy agent images in ordinary Claude Code, Codex and OpenCode sessions, with no daemon.
sidebar:
  label: tbox
  icon: box
---

`tbox` (`tbox/`, built to `bin/tbox`) is a standalone binary that builds a
Tariboy agent image from a source directory and starts an ordinary interactive
session of Claude Code, Codex or OpenCode with the image's prompt and skills.
There is no daemon, mail, task loop, AI proxy, tmux or budget. It is a separate
Go module and imports no Tariboy package, but it ships with every Tariboy
release and carries the Tariboy version: `tbox version` prints the same value
as `tariboy version`.

Image sources are local directories or Stores, and a v3 Tariboyfile may pull
skills and prompts from `git` and `url` sources. `tbox image new` assembles a
source from the skills and prompts of your Stores, and `tbox run` without
arguments lets you pick the image and the harness from lists.

## Install

```bash
make build
./bin/tbox version
```

`tbox` is installed together with `tariboy`, `ttasks` and `tmail`, at the same
version. `make install` copies `bin/tbox` next to the other binaries. On a
server, the release archive, `tariboy update`, the Desktop host install and
`make server-install` put it in the versioned release directory under
`~/.local/lib/tariboy/<version>` and link `~/.local/bin/tbox` to it, with the
same checksums and rollback as the other binaries. The Desktop package carries
it as well, and **Install/Update CLI** links it. `tbox` has no alias. It needs
no Node.js and no running Tariboy. To run an image you need at least one
supported harness on `PATH`:

- Claude Code 2.1.227 or newer, the first version with `--plugin-dir`
  (verified on 2.1.325). `tbox` compares the output of `claude --version`.
- Codex 0.166.0 or newer. `tbox` compares the output of `codex --version`.
- OpenCode: there is no version comparison. Before each start `tbox` runs
  `opencode debug config` against the generated configuration and requires
  the image's skills path in the result. The contract was verified on
  OpenCode 1.19.5, and a failed check reports that version as the minimum.

An unsupported harness is an error that names the minimum version, before
anything starts. `tbox` has no fallback route (such as a skill catalog in the
prompt) for a harness whose native skill route does not work. Each version
and configuration probe is stopped after 10 seconds, so a hanging harness
binary cannot block `tbox run`.

## Commands

| Command | Does |
| --- | --- |
| `tbox build [--path DIR] [--name NAME] [--tag TAG]... [--update]` | Build the image of a source directory; without `--path`, of the current directory. |
| `tbox build STORE/IMAGE [--name NAME] [--tag TAG]... [--update]` | Build an image of a Store. |
| `tbox images` | List the built images, then the images of the Stores. |
| `tbox image show REF`, `tbox image rm REF` | Show or remove a built image. |
| `tbox image export REF [-o FILE] [--force]` | Write a portable archive. |
| `tbox image import FILE [--force]` | Install a portable archive. |
| `tbox image new [NAME] [--skill STORE/SKILL]... [--prompt STORE/PROMPT]... [--new-prompt] [--version V] [--force] [--no-build]` | Create an image source from Store skills and prompts, then build it. |
| `tbox run [IMAGE [HARNESS]] [-- ARGS...]` | Start a harness session; missing arguments are chosen from lists. |
| `tbox run IMAGE HARNESS [-- ARGS...]` | Start a harness session. |
| `tbox run STORE/IMAGE HARNESS [-- ARGS...]` | Build if changed, then start. |
| `tbox run --path DIR HARNESS [-- ARGS...]` | Build if changed, then start. |
| `tbox store add NAME SOURCE [--ref REF]`, `list`, `show NAME`, `refresh NAME`, `ref NAME REF`, `remove NAME` | Manage Stores. |
| `tbox skills`, `tbox skill STORE/SKILL` (outside a session) | List the skills of all Stores, or show one. |
| `tbox prompts`, `tbox prompt STORE/PROMPT` | List the prompts of all Stores, or print one. |
| `tbox clean [IMAGE]` | Remove run directories, harness files and untagged image archives. |
| `tbox skills`, `tbox skill NAME`, `tbox skill exec NAME SCRIPT [ARGS...]` (inside a session) | List and show the skills of the session image, or run a script. |
| `tbox version`, `tbox --version` | Print the version. |
| `tbox help`, `tbox --help`, `tbox -h` | Print the list of commands. |

`tbox help` prints the commands to stdout and exits with code 0; an unknown
command prints the same list to stderr and exits with code 2. `-h` or
`--help` after a command, for example `tbox run --help`, prints the usage of
that command to stdout. In `tbox build`, `tbox run`, `tbox store add`,
`tbox image show`, `rm`, `export`, `import` and `new`, and `tbox clean`, flags
may come before or after the positional arguments, and `--` ends the flags of
`tbox`: everything after it is positional, and in `tbox run` it goes to the
harness unchanged. The other commands take no flags and read their arguments
as given.

## The `$TBOX_HOME` layout

The root is `$TBOX_HOME`, by default `~/.tbox`.

```text
~/.tbox/
  stores.json                          registered Stores
  stores/<name>/                       clone or unpacked archive of a Store
  cache/sources/<hash of the address>/ cache of remote skills and prompts
  locks/<hash>.json                    lock files of read-only sources
  sources/<name>/                      image sources written by tbox image new
  images/<name>/refs/<digest>.tar.gz   built images
  images/<name>/tags/<tag>             pointer to a digest
  run/<digest>/<harness>/              run directory
```

`images/` follows the layout of the Tariboy image store, so the tag rules and
`latest` are the same. Besides the archives, `images/<name>/` keeps the source
fingerprint of the last build, the source directory it came from, and a
`.lock` file: publishing, importing, removing and pruning an image of one name
wait for each other. A run directory holds the unpacked image
(`image/`), the joined prompt (`PROMPT.md`) and the files generated for that
harness. `tbox` never writes into your working directory, with two
exceptions: `tbox image export` without `-o` writes the archive there, and a
build writes `tbox-lock.json` next to a Tariboyfile that has remote entries
(see [Remote entries](#remote-entries-and-tbox-lockjson)), which is your
working directory when you build it with a bare `tbox build`.

## Stores

A Store is a place that holds images, skills and prompts. Its content is
untrusted: `tbox` reads it and never executes anything from it. It never
modifies its own clones and unpacked archives under `$TBOX_HOME/stores`. The
only file it may create in a directory Store or in an image source directory
is `tbox-lock.json`, next to a Tariboyfile that has remote entries, when that
directory is writable and not inside `$TBOX_HOME/stores`.

```bash
tbox store add team https://github.com/example/team-store.git
tbox store add team git@github.com:team/store.git --ref v2
tbox store add local ./my-store
tbox store list
tbox store show team
tbox store refresh team
tbox store ref team main
tbox store remove team
```

| Source | `add` | `refresh` |
| --- | --- | --- |
| git: `https://`, `ssh://`, `user@host:path`, `owner/repo` (GitHub) | Clones into `stores/<name>/`. | Fetches the selected ref. |
| `https://` archive, `.tar.gz`, `.tgz` or `.zip` | Downloads and unpacks. | Downloads again and swaps the tree. |
| A directory, relative or absolute | Records the absolute path and reads it in place. | Changes nothing. |

`--ref` (branch, tag or commit) and `tbox store ref` apply to git Stores only;
`tbox store ref NAME ""` returns to the default branch. `store remove` deletes
the clone or unpacked archive that `tbox` made, never a directory Store and
never built images. A Store name is lower-case letters, digits, `.`, `_` and
`-`.

The inventory of a Store:

- images: `images/<name>/Tariboyfile.yaml`;
- skills: `skills/<name>/SKILL.md`. When there is no `skills/` directory,
  `tbox` looks in `SKILL.md` at the Store root, `.agents/skills/` and
  `.claude/skills/`;
- prompts: `prompts/<name>.md`.

`workflows/` and everything else is ignored. A broken image, skill or prompt
is listed with its error and does not hide the others. Symlinks below the Store
root are not followed.

Items are addressed as `<store>/<name>`: `tbox build team/demo`,
`tbox skill team/review`, `tbox prompt team/process`. A bare name is an
error that lists the matching `<store>/<name>` candidates. `tbox skill
STORE/SKILL` prints the description, the sorted list of files and `SKILL.md` (for a skill at the Store root, only the files directly in the root);
`tbox prompt STORE/PROMPT` prints the file (at most 4 MiB). When stdout is a
terminal, control characters in `SKILL.md` and in the prompt other than
newline and tab are shown in caret notation (`^[`), and C1 controls and
invalid UTF-8 as `�`; written to a pipe or a file, the bytes are unchanged.
All other text from a Store, such as names and descriptions, is printed with
control characters replaced.

An image directory whose name is not a valid image name (for example one with
upper-case letters) is listed, but `tbox build STORE/IMAGE` then needs
`--name`.

`tbox images` ends with a table `STORE/IMAGE  VERSION  STATUS`. The status is
`built` when a built image of that name has the same `image_version`,
`not built`, or the error of the row.

### Official Store

The public Tariboy Store was verified with `tbox`: all six of its images built.
Two warnings appeared: one for images that declare `plugins`, and one for a
skill whose upstream moved since `skills-lock.json` was written (its computed
hash differs from the recorded one). These images contain skills that need the
Tariboy daemon; they build and run in `tbox`, but those skills do nothing
useful without it.

## Create an image from Store skills

`tbox image new` writes a source directory and builds it, so you can combine
skills and prompts of different Stores without writing a Tariboyfile by hand.

```bash
tbox image new review --skill team/review --skill team/security --prompt team/process
tbox image new notes --skill team/review --new-prompt --version 0.2.0
tbox image new                 # interactive: name, skills, prompts
```

Flags: `--skill STORE/SKILL` and `--prompt STORE/PROMPT` repeat, in the order
given; `--new-prompt` adds an empty `instructions.md` after the prompts (it is
added anyway when no prompt is selected); `--version` sets `image_version`
(default `0.1.0`); `--force` replaces an existing source; `--no-build` only
writes the source. Without `--skill`, `--prompt` and `--new-prompt`, the
command is interactive. Inside a session it is rejected like every other
build command.

The source is written to `$TBOX_HOME/sources/<name>/` and nowhere else; `tbox`
never writes into a Store. The `Tariboyfile.yaml` is schema v3, and the source
never names a Store, so it keeps building after the Store is removed or
renamed. Each entry is addressed by where the Store keeps the item:

| Store kind | Skill entry | Prompt entry |
| --- | --- | --- |
| git | `git` (the Store address) plus `path` and `ref` (the commit checked out) | the same |
| directory | `dir`, the absolute path | `file`, the absolute path |
| archive | a copy under `skills/<name>/` in the source | a copy under `prompts/<name>.md` |

A skill at the root of a directory Store is copied into the source too. A
root-level skill is the whole Store root except dot-directories and a root
`evals/` directory, so `tbox image new` refuses it when the root also holds
an `images/` directory or a `skills/<name>/SKILL.md`.
Copied files are checked with the same limits as a build. To change an image,
edit the files in `sources/<name>/` and run `tbox build --path
$TBOX_HOME/sources/<name>` (the default `~/.tbox/sources/<name>`). An entry
from a git Store pins the commit the Store had checked out, in `ref`; edit
`ref` to move it.

### Interactive form

With a terminal, `tbox image new` asks the name (when `NAME` is absent; an
invalid name, or the name of an existing source without `--force`, is asked
again with the reason), then
shows a checklist of every Store skill (`<store>/<skill>` and its
description), then a checklist of every Store prompt whose first row is
`new empty prompt (instructions.md)`. Rows with a problem are left out. When
the Stores hold no skills and no prompts the command exits with code 1 and
`no skills or prompts found; add a Store with: tbox store add NAME SOURCE`.

Keys in every list:

| Key | Does |
| --- | --- |
| Up, Down (or `k`, `j` while the filter is empty) | Move. |
| PageUp, PageDown | Move by a page. |
| Space | Toggle a row (checklists only). |
| Enter | Confirm. |
| Esc, `q` (while the filter is empty), Ctrl-C | Cancel. |
| Any other character, Backspace | Type into the filter, or delete from it. |

Cancelling exits with code 130 and writes nothing. The terminal is put back
exactly as it was before any build output is printed and before the harness
starts, and also when `tbox` is ended by `SIGTERM`, `SIGHUP`, `SIGQUIT` or
`SIGINT` while a list is shown. Text from Stores shown in the lists has
control characters replaced. The lists need a terminal on both stdin and
stderr. Without one, `tbox image new` without the flags exits with code 2
and names `--skill`, `--prompt` and `--new-prompt`; it never waits. A `NAME`
given on the command line is checked before any list is shown.

Security rules: a URL with user-info is rejected, as is a git ref that is empty,
starts with `-`, holds `..`, a space or a control character, or is longer than
255 characters. `git` runs with an argument array, a time limit, no prompts and
no hooks; the time limit is `TBOX_GIT_TIMEOUT` seconds, 120 by default.
`http://`, `file://`, `ext::` and local paths are not accepted as git
or archive addresses. An archive is unpacked with regular files and directories
only, no symlinks, hardlinks, absolute or `..` paths, at most 4096 members, 64
MiB compressed, 256 MiB unpacked and 512 bytes per path; a single top-level
directory is stripped. A failed `git` command is reported with the last line
of its stderr, with credentials masked; no other `git` output is shown.

## Build and change detection

```bash
tbox build --path ./my-image
```

The source is a directory with a `Tariboyfile.yaml` of `schema_version` 2 or 3.
`tbox build` without arguments builds the current directory. `tbox build
STORE/IMAGE` uses `images/<image>` of the Store; `--path` and a selector
exclude each other. A Store image is confined to its Store: an `extends`
layer, a local skill directory or a local prompt file whose path, absolute or
relative, leads outside the Store root is a build error; an absolute path
inside the Store is accepted. Remote entries are not confined. The name
defaults to the directory name, so to the
image name for a Store image, not `store/image`. Without `--tag` the image is published
under its `image_version` and under `latest`. The archive uses the Tariboy
schema v2 format byte for byte, so Tariboy imports it.

An image name is lower-case letters, digits, `.`, `_` and `-`, and its first
character is a letter or a digit. A tag uses the same characters, or is a
SemVer version, and does not start with `.`. This is stricter than Tariboy,
which also accepts a name that starts with `.`, `_` or `-`; a Tariboy export
with such a name cannot be imported into `tbox`. An image with such a name
that an earlier `tbox` built is still listed by `tbox images`, and
`tbox image show`, `tbox image rm` and `tbox clean IMAGE` accept its name. A
name that starts with `-` goes after `--`, for example
`tbox image rm -- -x:latest`.

`tbox` builds only when the source changed. Before each build it computes a
fingerprint over the Tariboyfile, the `extends` layers and every local skill
and prompt file that goes into the image. If it equals the fingerprint of the
last build and the image is still stored under every requested tag, nothing is
built and `tbox` prints `up to date`. The `image_version` number does not matter.
When the content changed under an unchanged `image_version`, the version tag and
`latest` move to the new build, and `tbox` prints one line saying so. This is
different from Tariboy, which refuses to rebuild the same version with other
content. The previous archive no longer has a tag and stays in `refs/` until
`tbox clean` removes it. `tbox image rm` removes the archive of the tag it
deletes when no other tag of the image points at it.

A source that uses `extends` builds to a different digest in `tbox` and in
Tariboy: Tariboy copies the layers into one directory, while `tbox` resolves
each entry in its own layer. In a child layer, an `only: tariboy` skill
replaces a parent skill of the same name, so `tbox` then packages neither.

Two Stores can hold an image of the same name. They build into the same image
name, and the later build moves the tags. `tbox` then prints the `moved` line
and `note: image <name> now comes from <store>/<image>`; use `--name` to keep
both. A build that runs while `tbox store refresh` replaces the same Store may
fail; run it again after the refresh.

### Remote entries and `tbox-lock.json`

In a v3 Tariboyfile a skill is exactly one of `dir`, `git` or `url`, and a
prompt one of `file`, `git`, `url` or `runtime`:

```yaml
schema_version: 3
skills:
  - git: https://github.com/obra/superpowers
    path: skills/brainstorming
    ref: v5.0.0
  - url: https://example.com/some-skill.tar.gz
prompts:
  - git: https://github.com/company/prompts
    path: process/review.md
  - url: https://example.com/review.md
```

`git` needs `path`; `ref` is a branch, tag or commit, and without it the default
branch is used. A skill `url` is an `https://` archive with the skill at its
root; a prompt `url` is one Markdown file. Remote content is cached in
`cache/sources/`.

`tbox` keeps a local mirror of each git repository with its branches and tags
only, so a commit reachable only from other refs (for example a pull-request
ref) cannot be pinned. A pinned commit that the mirror already holds needs no
network. A skill is exported with `git archive`, so the `.gitattributes`
in the repository (`export-ignore`, `export-subst`) apply to it, while your
`core.autocrlf`, `core.eol` and global attributes file (`core.attributesFile`)
are ignored; a prompt is read as the stored file. A skill at the repository root
(`path: .`) is the whole root except its dot-directories and `evals/`; a lock
record for such a skill made by an older `tbox` build may no longer match and
then needs `tbox build --update` once.

The first build pins every remote entry in `tbox-lock.json`: the address, the
commit (git) and the content hash. Later builds take the commit from the file.
The lock file is written next to the Tariboyfile when that directory is
writable and is not inside `$TBOX_HOME/stores`; otherwise it goes to
`$TBOX_HOME/locks/`. `tbox build --update` resolves every remote entry again,
rewrites the lock file and prints an `updated <address> <old>..<new>` line per
changed commit.

The change check never uses the network: remote entries count by their lock
records and the cache. The network is used only by `tbox build --update`,
`tbox store add`, `tbox store refresh` and the first resolution of an entry
that has no lock record. Offline, an unchanged image still runs; a build that
needs an uncached remote entry fails with the entry named.

### `skills-lock.json` in v2 images

A schema v2 image can come from the `npx skills` tooling, with
`skills-lock.json` next to the Tariboyfile or at the Store root and no
`.agents/skills/` in the checkout. `tbox` restores the `github` entries itself
into its cache (`cache/`) and reads the `local` entries in place, inside the
Store for a Store build; it does not change the Store. For a `--path` build, an
`.agents/skills/<name>` directory that is present on disk is used as it is;
for a Store build the skills are always restored from the lock. An `extends`
layer outside the image directory gets no `.agents/skills` remapping. It
supports `github`
sources (`source` plus `skillPath`, fetched at the default branch since the v2
lock holds no commit) and `local` sources. A computed hash that differs from
the content is a warning naming the skill, not an error. An image that declares
`plugins` also gets a warning, because some of its skills may need the Tariboy
daemon.

Warnings go to stderr, one per line, prefixed `warning: `.

## Run

```bash
tbox run my-image claude
tbox run my-image:0.2.0 codex -- --model gpt-5
tbox run team/demo claude
tbox run --path ./my-image opencode
```

`IMAGE` is a built image, `name` or `name:tag`; the tag defaults to `latest`.
`--path DIR` replaces it: `tbox` builds the directory if its content changed
and runs the result. A name that contains `/` is a Store selector: `tbox` builds
`STORE/IMAGE` if its content changed and runs the image under its image name.
A second run without changes prints no `built` line. `HARNESS` is `claude`,
`codex` or `opencode`. An unknown harness exits with code 2 and lists the known
ones. A missing image exits with code 1 and lists the built images. Everything
after `--` goes to the harness unchanged.

With fewer arguments than it needs, `tbox run` asks for the rest on a terminal:

- `tbox run` lists the built images (`<name>:<tag>`, with version and short
  digest), then the images of the Stores (`<store>/<image>`, with version and
  `built` or `not built`; rows with an error are left out), and then the
  harnesses;
- `tbox run IMAGE` asks only for the harness;
- `tbox run --path DIR` asks only for the harness.

The harness list shows the installed harnesses only. Whether one is installed
is checked before the image list is shown. When exactly one is installed it is
taken without a list and a line `using harness <name>` is printed. The keys
are those of the lists above. Exit codes: 130 on cancel; 1 when there is no
image (`tbox build` and `tbox store add` are named) or no harness installed
(the supported ones are listed); 2 without a terminal on stdin and stderr,
with `tbox run: IMAGE and HARNESS are required when stdin or stderr is not a
terminal`.

`tbox run` unpacks the image into `run/<digest>/<harness>/image/` if that is not
done yet, joins the static prompt layers into `PROMPT.md`, generates the
harness files, prints notes to stderr, and then **replaces itself** with the
harness process in the current directory and terminal. `tbox` does not stay in
the process tree. The harness gets your full environment, with
`TBOX_IMAGE_DIR=<run directory>/image` and `TBOX_IMAGE=<name>:<tag>` added; an
entry of the same name in your environment is replaced.

| Harness | Skills | Prompt | Written outside `$TBOX_HOME` |
| --- | --- | --- | --- |
| Claude Code | A generated plugin in the run directory, passed with `--plugin-dir` | `--append-system-prompt-file PROMPT.md` | Nothing |
| Codex | A plugin in the Codex plugin cache, enabled by a profile | `developer_instructions` in the profile | The plugin cache entry and the profile file, see below |
| OpenCode | `skills.paths` in a generated `opencode.json` | `instructions` in the same file | Nothing |

An image with no skills gets no plugin or skill path, and an image with an empty
prompt gets no prompt flag.

### Claude Code

`tbox` writes `plugin/.claude-plugin/plugin.json` and a copy of the skills into
the run directory and starts `claude --plugin-dir <run>/plugin
--append-system-prompt-file <run>/PROMPT.md` plus your arguments. Your Claude
settings are not touched.

### OpenCode

`tbox` writes `opencode.json` into a config directory under the run directory
and sets `OPENCODE_CONFIG_DIR` to it for the session. The file holds the
absolute path of the image skills in `skills.paths` and the absolute path of
`PROMPT.md` in `instructions`. OpenCode merges this directory with your global
configuration; `tbox` does not edit it. If `OPENCODE_CONFIG_DIR` was already set,
the launch replaces it and a note says so. Before starting, `tbox` runs
`opencode debug config` against the generated directory and refuses to start if
the skills path is not in the resolved configuration.

### Codex

`tbox` never modifies `$CODEX_HOME/config.toml` and never changes the
`CODEX_HOME` of your session. Codex has no per-session flag for a plugin, so
`tbox` writes exactly two things under `$CODEX_HOME` (default `~/.codex`):

- `plugins/cache/tbox/<plugin>/<version>/`, a plugin with
  `.codex-plugin/plugin.json` and a copy of the skills, next to a
  `.tbox-digest` record of the image and digest it holds and a `.lock` file in
  `plugins/cache/tbox/`;
- `tbox-<plugin>.config.toml`, a profile that holds `developer_instructions`
  with the prompt and enables the plugin with
  `[plugins."<plugin>@tbox"] enabled = true`.

`<plugin>` is the image name with every character outside `a-z0-9-` replaced
by `-`; `<version>` is the image version, or the first 12 characters of the
digest when the image has no version. `tbox` starts `codex --profile
tbox-<plugin>` plus your arguments. Without that flag Codex does not see the
plugin, so your ordinary sessions are unaffected. Skills appear as
`<plugin>:<skill>`. `developer_instructions` adds the image prompt to the
developer message and keeps Codex's own instructions. A cache entry is reused
when its record matches the image and digest, and replaced otherwise. Two image
names that map to the same plugin name share one entry; `tbox` prints a note
when it replaces the other image's entry. `tbox` refuses to write through a
symlinked `plugins/cache/tbox`.

While a launch prepares these files, it also creates short-lived staging
entries under `$CODEX_HOME`: `plugins/cache/tbox/.plugin-*` directories and
`tbox-<plugin>.tmp-*.config.toml` files. They are renamed into place or
removed; ones left by an interrupted launch are removed by the next launch and
by `tbox clean`.

Consequences of the shared cache:

- A Codex session started from an older build of an image loses its skills
  path when a newer build of the same image is run, because the cache entry is
  replaced.
- Image names that differ only in `.`, `_` and `-` (`a.b`, `a_b`, `a-b`) map
  to the same `<plugin>` and share the cache entry and the profile, and
  `tbox clean a.b` removes the Codex files of `a-b` too.
- `tbox` passes its own `--profile tbox-<plugin>`; a `--profile` of yours after
  `--` conflicts with it.

### Clean up

```bash
tbox clean            # all images
tbox clean my-image   # one image
```

`tbox clean` removes the run directories and, for Codex, the
`plugins/cache/tbox/<plugin>` entry, the leftover temporary profiles, and the
`tbox-<plugin>.config.toml` profiles whose first line is the
`# Written by tbox` header. A `tbox-*.config.toml` without that header is kept,
and a note names it. It removes nothing else: `config.toml` and the rest of
`$CODEX_HOME` stay byte for byte as they were. It also removes the archives in
`images/<name>/refs/` that no tag points at any more, such as the previous
build of a moved tag, and, when it removed any, prints how many. Tagged images stay;
remove those with `tbox image rm`.

`tbox clean IMAGE` finds the run directories of the image by the manifest of
the unpacked image. A run directory whose image cannot be read is kept, and a
note names it; `tbox clean` without an image removes it.

## Inside a session

The harness environment carries `TBOX_IMAGE_DIR`. While it is set, `tbox`
switches to agent mode and shows only the image of the session:

```bash
tbox skills                           # name and description of each skill
tbox skill NAME                       # description, files and SKILL.md
tbox skill exec NAME SCRIPT [ARGS...] # run a script inside the skill
tbox version                          # the tbox version
tbox help                             # these commands
```

Names, descriptions and file names in the output have control characters
replaced; the `SKILL.md` text is printed as it is.

`skill exec` runs a file inside the skill directory; the path cannot leave it.
The working directory stays the current one, `TBOX_SKILL_DIR` is added to the
environment, and the exit code and streams of the script pass through. Every
other command, including build and run, exits with code 2 and an error that
names the image of the session (or its directory when `TBOX_IMAGE` is not set)
and lists the commands above. If
the directory named by `TBOX_IMAGE_DIR` is missing or broken, `tbox` fails
instead of falling back to normal mode.

## What `tbox` ignores

`tbox` builds for use in a plain harness, so it drops what only the Tariboy
daemon uses:

- `plugins` in a Tariboyfile are accepted but not executed. The archive manifest
  carries `"plugins": []`, otherwise Tariboy would require the skills of those
  plugins on import.
- Runtime prompt entries (`runtime: workdir`, `runtime: messages`, and so on) are
  ignored and nothing replaces them. The session prompt is the static prompt
  layers of the image. An imported image that holds runtime entries runs the
  same way, with the entries skipped.
- In schema v3, a skill or prompt entry with `only: tariboy` never enters the
  archive or the fingerprint. One source therefore yields different archives in
  `tbox` and in Tariboy.
- A `$PLUGINS/...` path needs a plugin directory that `tbox` does not have. In
  v3 it is accepted only with `only: tariboy`; in v2 the build fails and names
  the entry.

## Export and import with Tariboy

```bash
tbox image export my-image:1.0.0 -o my-image.tariboy-image.tar.gz
tbox image import my-image.tariboy-image.tar.gz
```

The file is the `tariboy-portable` envelope that `tariboy image import`
accepts, so an image built in `tbox` imports into Tariboy, and `tbox` imports
the export of Tariboy. A portable image whose name differs from its envelope ref
is rejected. When the image already has the tag with other content, the import
fails and names the ref; `--force` moves the tag to the imported content. An
image imported from Tariboy runs as a whole; skills that need
the daemon are still in it. Compatibility is covered by shared fixtures in
`tbox/testdata/compat/`, and a format change in Tariboy breaks those tests.

## Known limits

- Global skills of the user stay visible to every harness next to the skills of
  the image. `tbox` does not isolate the session.
- Tariboy does not understand `schema_version: 3` yet, so a v3 source builds in
  `tbox` only. The archive that `tbox` writes is schema v2.
- The harness adapters were verified in live sessions against Claude Code
  2.1.325, Codex 0.166.0 and OpenCode 1.19.5: in each one the model quoted a
  marker from the prompt of the image, and in Claude Code and Codex it named a
  skill of the image as `<image>:<skill>`. Claude Code was checked in an
  interactive session and with `-p`, Codex with `codex exec`, OpenCode with
  `opencode run`. The interactive screens of Codex and OpenCode read the same
  profile and configuration, but nobody drove them through `tbox` yet. It is
  also not known whether Codex ever prunes a plugin cache entry that no
  marketplace lists.
- Whether a skill is invoked depends on the harness and the model.
