## [0.62.1] - 2026-09-15

### Fixed

- Make repository and release checks portable to macOS by accepting canonical image source roots through system symlinks, enforcing Darwin Unix-socket limits, handling non-terminal shim probes, and supplying test-only installer command fallbacks without weakening production requirements.

[0.62.1]: https://github.com/alekzonder/tariboy/compare/v0.62.0...v0.62.1

## [0.62.0] - 2026-09-15

### Added

- Organize the console sidebar into Agents, Groups, and Servers tabs, with pinned agents, unread-question priority, cross-server group membership, and server update actions.
- Allow trusted image-creator agents to build images from any source directory readable by the daemon.

### Changed

- Let builds, imports, retags, and registry publication advance any non-reserved image tag while retaining prior archives by digest for pinned assignments.
- Run the repository checks before packaging a tagged Desktop release.

### Removed

- Remove the Evals subsystem and its image manifest, plugin, CLI, API, UI, and runtime surfaces while retaining legacy persisted data for safe upgrades.
- Remove AI proxy Rules, including model routing, model allow/deny policy, rate limits, and their CLI, API, UI, and runtime surfaces.

### Fixed

- Flush queued AI usage before terminal iteration span aggregation.
- Hide stale image provenance and reject stale team snapshots after a tag advances to different image bytes.

[0.62.0]: https://github.com/alekzonder/tariboy/compare/v0.61.0...v0.62.0

## [0.61.0] - 2026-09-14

### Added

- Retry Store image builds after immutable-tag conflicts with an alternative target image name or tag.
- Show dependency titles and statuses in task details.

### Changed

- Simplify the task detail panel around the compact operator template.

### Removed

- Remove Judge reviews, automation, controlled improvements, and their CLI, API, UI, documentation, and persisted state.

### Fixed

- Reject immutable Store image targets before restoring skill locks, release build controls promptly, and prevent stale inventory reads from replacing newer build results.
- Allow Goal wake delivery to recover after an earlier delivery reaches the dead-letter queue.

[0.61.0]: https://github.com/alekzonder/tariboy/compare/v0.60.0...v0.61.0

## [0.60.0] - 2026-09-14

### Added

- Copy the original Markdown from task comments.
- Browse image registry names and tags with coherent details for the selected image version.
- Show each agent’s active image version and the next available image version.
- Filter console agents by agent name or server label.

### Changed

- Redesign the console shell, task table, task detail panel, and application theme around the denser metal-and-blue operator interface.
- Publish documentation automatically after a successful Desktop release.

### Fixed

- Preserve unsupported Markdown content when editing task text in the rich editor.
- Accept `argument-hint` in Agent Skill frontmatter.
- Report latest-image inspection errors and retain existing agent image status when inventory refresh fails.
- Use English labels in application settings.
- Keep task detail controls usable at narrow widths and preserve Queue settings labels.

[0.60.0]: https://github.com/alekzonder/tariboy/compare/v0.59.0...v0.60.0

## [0.59.0] - 2026-09-11

### Added

- Connect a saved SSH host to an existing Tariboy daemon without installing, starting, or restarting remote software.

### Fixed

- Preserve the selected SSH setup mode while retrying an unavailable daemon or falling back to installation.

[0.59.0]: https://github.com/alekzonder/tariboy/compare/v0.58.1...v0.59.0

## [0.58.1] - 2026-09-11

### Fixed

- Restore Desktop builds after adding the application updater.

[0.58.1]: https://github.com/alekzonder/tariboy/compare/v0.58.0...v0.58.1

## [0.58.0] - 2026-09-11

### Added

- Download, signature-verify, and explicitly install Desktop updates from application settings.
- Present one-shot instructions, messages, and Native Task goals in a platform-owned processing order for every non-bare agent.

### Changed

- Publish the checked DMG together with the signed updater archive, signature, and update catalog through an atomic draft-to-public release flow.
- Prepare locked Go, UI, and documentation dependencies automatically before fast checks.

### Fixed

- Compare Desktop update versions by SemVer precedence.
- Preserve Markdown headings in Native Task questions.
- Remove the release artifact scanner’s ripgrep runtime dependency.

[0.58.0]: https://github.com/alekzonder/tariboy/compare/v0.57.5...v0.58.0

## [0.57.5] - 2026-09-11

### Fixed

- Avoid false credential detections while verifying macOS release artifacts.

[0.57.5]: https://github.com/alekzonder/tariboy/compare/v0.57.4...v0.57.5

## [0.57.4] - 2026-09-11

### Fixed

- Publish macOS release artifacts without requiring Linux binary emulation.

[0.57.4]: https://github.com/alekzonder/tariboy/compare/v0.57.3...v0.57.4

## [0.57.3] - 2026-09-11

### Fixed

- Use the installed QEMU runner when verifying Linux x86_64 release binaries on macOS.

[0.57.3]: https://github.com/alekzonder/tariboy/compare/v0.57.2...v0.57.3

## [0.57.2] - 2026-09-11

### Fixed

- Publish signed macOS artifacts without running the isolated desktop smoke suite.

[0.57.2]: https://github.com/alekzonder/tariboy/compare/v0.57.1...v0.57.2

## [0.57.1] - 2026-09-11

### Fixed

- Validate schema-v2 editable image sources before building them.

[0.57.1]: https://github.com/alekzonder/tariboy/compare/v0.57.0...v0.57.1

## [0.57.0] - 2026-09-11

### Added

- Run eligible saved scripts immediately, including idle recurring scripts.
- Mark all unread task notifications as read from the Tasks inbox.
- Register editable image-source API routes.

### Fixed

- Confine agent-authored image builds to each agent's managed workdir.
- Register editable image-source routes.

[0.57.0]: https://github.com/alekzonder/tariboy/compare/v0.56.0...v0.57.0

## [0.56.0] - 2026-09-11

### Added

- Agent and global shell-script editors, validated and sourced before harness launch.
- Image version visibility and Store image update status.
- Persistent, keyboard-accessible sidebar ordering for servers, teams, and agents.
- Stalled AI-iteration reporting and configurable stall timeout.
- Tag-triggered macOS release publication.

### Fixed

- Shell-script loading and launch-argument preservation.
- Audit-export streaming and transcript deduplication.
- Task and goal selection with pull-request URLs.

[0.56.0]: https://github.com/alekzonder/tariboy/compare/v0.55.0...v0.56.0
