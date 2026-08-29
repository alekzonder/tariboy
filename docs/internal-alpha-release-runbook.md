# Tariboy internal alpha release runbook

Release: `0.45.0`

## Ownership

- Release owner and incident lead: repository owner
  [`alekzonder`](https://github.com/alekzonder).
- Backup reviewer: a repository collaborator other than `alekzonder`, recorded
  as the second approver on the release issue before publication.
- Support intake: the repository issue system, restricted to non-sensitive
  summaries. Support ZIPs travel only through an approved internal channel.

The Git remote establishes the repository owner. It does not establish an
organization-specific artifact host or upload protocol.

## Release prerequisites

- clean `main` worktree at the reviewed commit;
- Apple Silicon Mac with macOS/Xcode tools, Go 1.26, Rust, Tauri CLI, Node, and
  `rg`;
- system `codesign`, `hdiutil`, `lipo`, `shasum`;
- a way to execute Linux x86_64 static binaries during verification:
  `qemu-x86_64`, a locally cached `alpine:3.22` Docker image, or an approved
  executable set in `TARIBOY_LINUX_AMD64_RUNNER`;
- no production credentials in build environment or repository.

## Set the release version

1. Decide the new version: use patch or minor based on everything landed since
   the previous release.
2. Run `scripts/set-version.sh NEW_VERSION`. It rewrites the allowlisted version
   files, writes `scripts/release-version.txt`, and regenerates
   `desktop/src-tauri/Cargo.lock` through Cargo.
3. Run the release gates:

   ```bash
   . "$HOME/.cargo/env"
   make desktop-version-check
   make desktop-lock-check
   go test ./scripts/...
   ```

4. Complete the build, package, and verification steps below.

After step 2, this runbook and other documentation may name artifacts, such as
the versioned DMG, that do not exist yet. That is expected pre-existing
behavior: the packaging step creates them.

## Build

```bash
git status --short
git rev-parse HEAD
. "$HOME/.cargo/env"
make desktop-alpha
```

The target builds both binary platforms and the SPA, creates ad-hoc-signed app
and DMG bundles, verifies signatures, runs the isolated desktop smoke test, and
stages:

```text
dist/releases/0.45.0/
  Tariboy_0.45.0_aarch64.dmg
  SHA256SUMS
  release.json
```

DMG creation intentionally uses Tauri's headless-safe mode. It skips Finder
AppleScript layout automation, so release packaging does not require an
Automation permission grant; the application, `Applications` link, signatures,
and artifact checks are unchanged.

Re-run the independent gate:

```bash
scripts/check-alpha-artifacts.sh dist/releases/0.45.0
```

## Two-person review

The release owner and backup reviewer each verify:

- [ ] commit SHA in `release.json` is the reviewed `main` commit;
- [ ] artifact gate exits zero on Apple Silicon;
- [ ] `shasum -a 256 -c SHA256SUMS` exits zero;
- [ ] release notes and known constraints match the artifact;
- [ ] support/rollback contacts and process are understood;
- [ ] a disposable-host product acceptance run is attached;
- [ ] no live user host or base directory was used for acceptance.

Both approvers record their GitHub usernames and timestamp on the release issue.

## Publication gate

Publication is blocked until an authorized release owner supplies and reviews
the exact internal HTTPS destination and upload semantics. Neither the GitHub
remote nor repository configuration provides that information, so this runbook
does not invent an upload URL, credential name, or `curl` command.

The approved command is recorded on the access-controlled release issue and
must:

- use HTTPS;
- upload exactly the three checked files;
- preserve the full versioned filename;
- avoid credentials in shell history and logs;
- be reviewed by the backup reviewer before execution.

GitHub Releases, public object storage, personal file sharing, and unreviewed
chat attachments are not substitutes for the approved internal destination.

After upload, a reviewer downloads into a new directory, verifies
`SHA256SUMS`, and compares `release.json` commit SHA before invitations are
sent.

## Design-partner rollout

Invite five to eight internal partners who regularly operate coding agents and
can use a disposable Linux x86_64 host. Send:

> Tariboy is one desktop workspace for coding-agent sessions across hosts.
> This alpha starts with familiar interactive terminals, then adds reusable
> images, bounded Autopilot, usage, and audit. We are looking for a ten-minute
> observed onboarding and one weekly 20-minute interview. The app has no
> built-in product analytics; optional OTLP is off unless you configure it.
> Please report issues directly and do not use production secrets or
> irreplaceable hosts during the alpha.

Use the [demo script](internal-alpha-demo-script.md) for kickoff and the
[feedback template](internal-alpha-feedback-template.md) after each session.

## Incident and rollback

1. Release owner disables further downloads at the internal destination.
2. Ask affected users to disable Autopilot; use Kill only for unsafe active work.
3. Preserve non-sensitive logs and export support bundles with user review.
4. Reinstall the previous verified DMG.
5. Update hosts so their remote daemon matches the rolled-back app.
6. Record affected version, commit, scope, and recovery in the repository issue.

Removing a host does not stop its remote daemon. If a remote daemon must stop,
run `tariboy daemon stop` on that host explicitly.

## Uninstall

Follow the ownership checks in the support guide before removing
`/Applications/Tariboy.app` or the managed `~/.local/bin/tariboy` link.
Stop daemons separately. Preserve `~/.tariboy` unless durable agent data is
intentionally being deleted.
