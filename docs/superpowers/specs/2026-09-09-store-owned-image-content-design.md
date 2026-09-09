# Store-Owned Image Content Design

Native Task: IMPROVE-43. Approved by customer comment 1720 on 2026-09-09.
Completion uses separate PRs for `tariboy-store` and `tariboy`. The customer
explicitly rejected a Git submodule and required `make check`, not
`make full-check`.

## Outcome

`tariboy-store` becomes the only source repository for ordinary agent images,
Agent Skills, and static image prompt files. Every schema-v2 runnable image
contains the complete immutable skill trees and static prompt bytes declared by
its source. `tariboyd` retains dynamic runtime rendering and built-in/external
plugin implementations, but no longer embeds or installs raw Store skills or
static image prompts.

The daemon continues to synthesize `bare:latest`, because bare is runtime
policy rather than an ordinary role. It no longer builds, embeds, or seeds
`basic:latest`; `images/basic` in `tariboy-store` is built and assigned like
every other versioned Store image.

## Repository boundary

The root of `tariboy-store` owns:

```text
images/
  basic/
  llm-as-judge/
  tariboss/
  tariboy-developer/
  tariboy-image-creator/
skills/
  loop/
    finish-iteration.md
README.md
Makefile
scripts/check-images.sh
```

Image manifests use repository-relative skill paths. The shared mandatory
finish prompt lives once at `skills/loop/finish-iteration.md`; every image that
packages `../../skills/loop` may reference that file through the same declared
skill's immutable snapshot. No manifest
uses `$STORE`, `$CURRENT_VERSION_STORE`, a tariboy checkout path, or a
submodule. Existing image versions receive a patch increment for the changed
source identity; `basic` starts at `0.1.0`.

`make check` in `tariboy-store` runs the moved Python skill contracts, rejects
forbidden path roots, restores locked dependencies in a temporary source copy,
rejects image-local finish-prompt copies, and validates/builds every
image through explicitly supplied `tariboy` and
`tariboyd` binaries against owner-only temporary base/runtime directories with
the HTTP listener disabled. The checkout remains unchanged.

## Image/runtime contract

A schema-v2 manifest declares three independent things:

- plugin names implemented by the target daemon;
- packaged Agent Skill directories copied into the runnable archive;
- ordered static prompt files and dynamic runtime placeholders.

Build and portable-image validation use one daemon-owned capability registry.
For each runtime placeholder the registry names its renderer and, where
applicable, owning built-in plugin and packaged skill. Built-in plugins record
whether a same-named packaged skill is required and whether that skill must
contain an executable launcher.

Validation fails on the daemon serving the build when:

- a declared built-in runtime placeholder has no daemon renderer;
- a declared plugin is neither built in nor installed and enabled;
- a skill-owned runtime placeholder omits its owning plugin;
- a built-in capability that requires instructions omits its packaged skill;
- `loop` or legacy `tasks` omits its executable packaged launcher.

Portable import and pending activation repeat the same contract check against
the destination daemon. An incompatible pending image is not promoted and the
old active image, bridge, and shims remain usable.

## Image-owned launchers

`internal/agentdir.WriteShims` resolves launchers below the unpacked active
image:

```text
<agent>/image/skills/loop/scripts/loop.sh
<agent>/image/skills/tasks/scripts/tasks.sh
```

The files must be regular and executable. Provision, startup reconciliation,
pending-image activation, rollback, and crash recovery all call the same
image-relative shim writer. Candidate shims are written only after candidate
image bytes are staged; any failure restores both old bytes and old shims.

## Removal and compatibility

Tariboy removes `store/images`, `store/skills`, `store/prompts`, package
`storeassets`, `$STORE`/`$CURRENT_VERSION_STORE` resolution for new schema-v2
sources, `ManagerConfig.SkillsDir`, `internal/builtinimages`, and build/startup
basic-image generation. Existing `<base>/store/versions/*` directories are
left untouched and simply become unread legacy data.

New schema-v1 source build/validate returns a migration error requiring schema
v2. Existing self-contained schema-v1 archives remain readable and runnable
only when their archived bytes contain required launchers; the daemon never
fills missing content from a hidden Store fallback.

## Safety and migration order

1. Add the narrow Tariboy resolver support for prompt files inside declared
   sibling skills, then verify the `tariboy-store` source against it.
2. Merge the `tariboy` and `tariboy-store` PRs after both are green.
3. Run the distinct `make check` on refreshed `main` in each repository.

No test starts or stops the live daemon, reads live Tariboy data, or binds
`127.0.0.1:9990`. No product version is changed by this ordinary task.

## Acceptance criteria

- `tariboy-store` root is the only source of ordinary images, skills, and
  static image prompt files.
- Every Store image validates and builds without `$STORE` or
  `$CURRENT_VERSION_STORE`.
- Tariboy contains no embedded raw skill/prompt bundle and seeds no
  `basic:latest`.
- Build, import, assignment, and activation reject unavailable plugins,
  runtime renderers, packaged skills, and required launchers.
- Agent compatibility shims execute bytes from the active image digest.
- Failed activation preserves the previous image and shims.
- `bare:latest` remains available and existing self-contained artifacts remain
  runnable within the stated schema-v1 boundary.
- There are exactly two PRs and both use `make check`; `make full-check` is not
  run.
