---
title: Compose decisions
description: What tariboy compose does in special cases - a partial failure, an unavailable Store, a version conflict, an entity in use, a removed entity, a second up, secrets, dry-run, versions and migration.
sidebar:
  label: Compose decisions
  icon: clipboard-check
---

This page gives one entry for each special case of `tariboy compose`. Each
entry gives the case, the decision, what you see, and the section of
[Compose](/docs/binaries/compose) with the details. `scripts/compose-e2e.sh`
runs most of these cases on an isolated daemon (see
[Development](/docs/development)).

## A failure in the middle of `up`

| | |
| --- | --- |
| Case | An action of `up` fails, for example a build, a create or a secret. |
| Decision | `up` does not stop and does not undo. It skips each action that depends on the failed action, in every later phase. Independent actions still run. |
| What you see | One line for each resource: `ok`, `changed`, `failed` with the first error, or `skipped`. `up` exits non-zero and prints `compose up: N resource(s) failed`. The daemon is partly converged. Correct the cause and run `up` again. |
| Details | [When an action fails](/docs/binaries/compose#when-an-action-fails) |

An error while `up` reads the live state or plans stops `up` before any
change.

## A Store that is not available

| | |
| --- | --- |
| Case | The daemon cannot reach a Store: no network, a wrong URL, a `ref` that does not exist, a path that does not exist, or another source under the same name. |
| Decision | The Store fails. Each workflow and image from it is `skipped`, and so are the agents on those images and the queue bindings of those workflows. Entries from paths and from other Stores still apply. `up` never changes the source of a registered Store. |
| What you see | `failed store:NAME` and `skipped` lines for its dependents. `up` exits non-zero. After you correct the Store, the next `up` applies the skipped resources. |
| Details | [A Store that is not available](/docs/binaries/compose#a-store-that-is-not-available), [Stores](/docs/binaries/compose#stores) |

## A version conflict

| | |
| --- | --- |
| Case | The `version` of a workflow or image entry differs from the version that its source gives. |
| Decision | The entry fails before the build, and its tag stays on the build it had. If the source changes during the build, the entry fails after the build. A source without a version fails too. |
| What you see | `failed image:NAME: image NAME: expected version X, the source gives Y; the tag is not moved`. `compose status` shows `version conflict: want=X source=Y`. |
| Details | [Builds and versions](/docs/binaries/compose#builds-and-versions) |

An `image_version` that the daemon already has with other content also fails
the build. `unpack` reports such a conflict as a failed import.

## An entity that is in use

| | |
| --- | --- |
| Case | `up` changes an agent that runs an iteration, or a queue with open tasks. |
| Decision | `up` does not wait. A new image is pending until the next iteration. A harness change restarts a running agent in the start phase and ends its iteration. Every other agent setting applies from the next iteration. A queue bound to a new workflow version keeps its started tasks on the version they pinned. |
| What you see | `changed agent:NAME`. `compose status` shows `image pending` until the agent adopts the image. |
| Details | [Entities that are in use](/docs/binaries/compose#entities-that-are-in-use), [When the settings apply](/docs/binaries/compose#when-the-settings-apply) |

## An entity removed from the file

| | |
| --- | --- |
| Case | You remove an agent, a group, a task queue, a pool, an env key, a secret or a trigger from the file. |
| Decision | `up` never deletes. The entity stays on the daemon unchanged. `compose down` before you edit the file, or a direct command, removes it. |
| What you see | `compose status` and `up --dry-run` list the entities that are tied to the file as `not declared`. These lines are not drift. |
| Details | [Removing an entity from the file](/docs/binaries/compose#removing-an-entity-from-the-file), [What `up` does not change](/docs/binaries/compose#what-up-does-not-change) |

## A second `up`

| | |
| --- | --- |
| Case | You run `up` again on a file that the daemon already matches. |
| Decision | A setting that matches makes no call. `up` refreshes each git Store and builds each path image and each workflow again, because it cannot tell whether a local source changed. A build of unchanged sources gives the digest that the daemon has. An image from a Store builds only for a new `image_version`. `up` re-provisions only a stopped agent that `compose down` left without its image tree. A stopped agent that has its image, such as one with `loop.enabled: false`, stays stopped and gets no call. |
| What you see | Each resource is `ok`, and the summary is `0 changed`. |
| Details | [A second `up`](/docs/binaries/compose#a-second-up), [How `up` converges](/docs/binaries/compose#how-up-converges) |

The first `up` after `down` starts an agent with `loop.enabled: false` and
stops it again, because a re-provision always starts the loop. The agent can
start an iteration between these two calls.

## Secrets

| | |
| --- | --- |
| Case | A queue or an agent needs a secret. |
| Decision | The file holds only the secret name and its `from_env` variable. `compose` reads the value from its own environment when it writes the secret. An empty variable counts as not set. A secret that is missing on the daemon and not set in the environment fails its queue or agent. `up` never removes a secret. |
| What you see | The value never appears in the output of `up`, `up --dry-run`, `status`, `pack` or `unpack`, in an error, or in a compose archive. `status` shows `secret KEY ok (value not compared)` or `MISSING`. |
| Details | [Secrets](/docs/binaries/compose#secrets), [Agent secrets](/docs/binaries/compose#agent-secrets) |

After `unpack`, a secret reaches the target daemon only if its `from_env`
variable is set in the environment of `unpack`.

## Preview with `--dry-run`

| | |
| --- | --- |
| Case | You want to see the plan before you apply it. |
| Decision | `up --dry-run` and `unpack --dry-run` read the live state with `GET` requests only. They do not start the daemon, build, refresh a Store or write. |
| What you see | The plan by phase, the `not declared` entities, and `dry-run: N resources to change, M ok; nothing applied`. Env and secret values are not printed. A build, a Store refresh and a Store version show as planned, because a dry run cannot know their result. |
| Details | [Preview with `--dry-run`](/docs/binaries/compose#preview-with---dry-run) |

## Versions

| | |
| --- | --- |
| Case | The file names the tariboy versions that it works with, or an archive comes from another daemon version. |
| Decision | `tariboy: {min, max}` holds inclusive SemVer bounds. Every verb that calls the daemon, `pack` and `unpack` included, reads the daemon version first and stops before any change when it is outside the range. `validate` compares the range with the CLI, and with `--daemon` also with the daemon. `unpack` and `validate --archive` also refuse a daemon or a CLI that is older than the `producer_version` of the archive. |
| What you see | `compose up: daemon version 0.109.0 does not satisfy tariboy >= 0.110.0 in /work/Tariboycompose.yaml`. The daemon does not change. |
| Details | [The tariboy version range](/docs/binaries/compose#the-tariboy-version-range), [`unpack`](/docs/binaries/compose#unpack) |

The `version` of a workflow or image entry is a different check: see
[A version conflict](#a-version-conflict).

## Migration from `version: 1`

| | |
| --- | --- |
| Case | You have a version 1 file. |
| Decision | A version 1 file works as before. To use `tariboy`, `stores`, `workflows`, or an image `path`, `store`, `source`, `tag` or `version`, declare `version: 2`. In version 2, write an image `context` as `path`, and declare each agent image and queue workflow in the file. |
| What you see | A version 1 file with a version 2 key fails with an error that tells you to declare `version: 2`. An empty `workflows: {}` from an older team export is accepted. |
| Details | [Version 1 and version 2](/docs/binaries/compose#version-1-and-version-2) |

## Older archives and the `archive` and `import` verbs

| | |
| --- | --- |
| Case | You have a team archive from an older `compose archive`, or scripts that call `compose archive` or `compose import`. |
| Decision | `compose archive` and `compose import` stay for one release as deprecated aliases of `pack` and `unpack`, with a warning. `compose unpack` also accepts a version 1 team archive and sends it to the team import of the daemon. The CLI does not write team archives any more. |
| What you see | A deprecation warning on standard error. For an old team archive, the team import preview, then the import after `--yes` or a `y` answer. |
| Details | [Older archives and the `archive` and `import` verbs](/docs/binaries/compose#older-archives-and-the-archive-and-import-verbs) |

To move such a team with its images, apply the old archive, then run
`compose pack` with the compose file of the team.
