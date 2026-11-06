---
title: Images & groups
description: Reproducible agent images, custom groups, and portable team transfers.
sidebar:
  label: Images & groups
  icon: boxes
---

## Images

An **image** is an explicit plugin list, optional packaged Agent Skills, and
an exactly ordered prompt template, built from an original directory containing
`Tariboyfile.yaml`. A ref's identity comes from the image name and its declared
`image_version`, and a tag is only a pointer to a ref.
Schema v2 does not contain runtime defaults or inherit another image. Static
prompt files are embedded during build; dynamic values remain ordered runtime
placeholders. Packaged skills travel with runnable image export/import and are
projected through harness-specific integration at iteration launch: native
configuration for Claude Code and OpenCode, and a compact prompt catalog of
absolute `SKILL.md` paths for Codex.
See [Images](/docs/images) for the complete schema and path roots.

Tariboy provides one reserved image and the official Store provides the usual
general-purpose source:

- **basic** — the general-purpose source in `tariboy-store` (context, status,
  managed-workdir instructions, scripts, Goal attribution, native Tasks, and
  `runtime: goal`). It deliberately excludes all external provider
  integrations. Register and refresh that Store, then build the source.
- **bare** — a daemon-synthesized empty schema-v2 plugin/prompt artifact whose
  terminal-only behavior is runtime policy.

## Groups

A **group** ties agents together with a designated **lead**. A letter to
`group:<g>` goes to every member except the sender; members ask one another
with letters to `agent:<member>`. Group members can share a working directory.
Native Tasks stays daemon-owned and applies live group membership when checking
task visibility.

## Historical group Usage

Group membership for AI Usage is intentionally historical rather than live.
Every new proxied request stores the agent's group identifier and display name
at request time. Moving an agent to another group, renaming a group, or deleting
it does not relabel earlier costs. Requests recorded without a snapshot,
including older rows, appear as **Ungrouped** instead of inheriting current
membership.

Open **Settings → Usage & budgets** for a host to filter all Usage
projections by **All groups**, **Ungrouped**, or a current group. The selection
applies together to summary totals, daily series, aggregate rows, and recent
requests. Saved names for deleted groups remain visible in historical rows,
but deleted groups are not added back to the selector. Agent-level Usage stays
available on the ordinary agent workspace and has no group selector. See [AI
proxy and audit](/docs/architecture/ai-proxy#group-snapshots-and-usage) for the
snapshot and query model.

## Team lifecycle and portability

Desktop presents grouped agents under **Teams** separately from **Individual
agents**. Each member is still an ordinary agent and has the same lifecycle,
terminal, configuration, and task controls. A custom team may contain any
number of differently configured images; members can be assigned or removed at
any time without deleting the underlying agent. Teams can be renamed and their
lead changed in **Settings → Teams**.

The creation wizard creates custom teams directly. Each member independently
selects its name, image, harness, model, effort, cwd, interactive mode,
environment, and external plugins.

**Copy YAML** emits `tariboy-compose.yaml` for text-only transfer. **Import
YAML** recreates the group and agents when referenced images already exist.
The portable team archive is likewise compose-only: it contains team/runtime
compose configuration, no `images:` build contexts, source trees, or runnable
image bytes. Transfer each runnable image separately through **Images →
Export/Import**. Keep original image directories and their paths when the
destination must rebuild rather than import an already built artifact. Team
import progress remains persisted per agent and stays pinned to the host on
which preview began.

For automation, the operator CLI moves a whole compose project, with its
runnable images and workflow images, in one compose archive. Both commands
need a running daemon:

```bash
tariboy compose pack -f ./Tariboycompose.yaml -o ./team.tar.gz
tariboy compose unpack ./team.tar.gz
```

`compose unpack` checks the whole archive before any change, imports the
images and workflow images without network access and without a build, then
applies the file. It prints the plan and asks for confirmation. Use `--yes` in
non-interactive automation only after you read the plan and the destination
settings. `compose unpack` also accepts a team archive that an older
`compose archive` wrote. `compose archive` and `compose import` are deprecated
aliases of `pack` and `unpack` for one release. See
[Moving a project with `pack` and `unpack`](/docs/binaries/compose#moving-a-project-with-pack-and-unpack).
