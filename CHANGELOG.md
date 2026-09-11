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
