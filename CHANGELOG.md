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
