# Mutable Agent Image Tags Design

## Goal

Allow an operator to rebuild an ordinary agent image tag, including `latest`,
and let agents already assigned to that mutable ref activate the new digest at
their next iteration boundary. Allow one CLI build to publish multiple tags by
repeating `--tag`.

## Publication contract

`tariboy image build` is the mutable authoring path. Each requested ordinary
ref is replaced atomically after the complete archive has been validated. The
previous archive is retained by digest before the ref moves, so running,
active, and pending assignments remain recoverable. `basic` and `bare` remain
reserved, and refs recorded as controlled-improvement releases cannot be
replaced through ordinary authoring.

Runnable import/retag, registry publication, and controlled-improvement
publication keep their existing immutable behavior. Mutable state is recorded
beside the ref in the image store; it is not inferred from tag spelling, so a
production tag and a mutable authoring tag can use the same ref grammar without
sharing lifecycle semantics.

## Multiple tags

The operator CLI accepts repeatable tags:

```bash
tariboy image build --path ./agent-image --name reviewer \
  --tag latest --tag v2
```

The source is parsed once. Every requested tag is validated before publication,
duplicates are rejected, and each result is returned with its own ref and
digest. A single `--tag` and an omitted `--tag` remain compatible with existing
callers; omission publishes `latest`. Static image content, source provenance,
and build time are shared, while each archive has its own required manifest ref
and therefore its own digest.

## Activation contract

At the existing per-agent launch gate, Tariboy first honors an explicit pending
assignment. Otherwise, when the active ref is marked mutable, it compares the
current ref digest with the agent's pinned digest. A changed digest is persisted
as pending and passed through the existing staging, skill-bridge, shim,
promotion, audit, and rollback path. The running iteration is never interrupted.

An unchanged ref does no extra work. Immutable refs, explicit controlled
rollouts, and retained historical digests remain pinned.

## Failure and recovery

- Replacement preserves the old archive before moving the ref.
- `InspectPinned` and `UnpackPinned` resolve retained mutable generations by
  digest, as they already do for daemon-managed generations.
- A validation, staging, bridge, or shim failure leaves the prior active digest
  and records the ordinary pending-image error.
- Reserved refs and controlled-release refs fail before publication.
- Test daemons use isolated base/runtime directories and never touch the live
  daemon.

## Acceptance criteria

1. Rebuilding `reviewer:latest` with changed source succeeds.
2. `--tag latest --tag v2` publishes both refs from one command.
3. An agent on the mutable ref keeps its current image while running and
   activates the new digest only at its next launch gate.
4. A prior active or pending digest remains inspectable and unpackable after a
   ref moves.
5. `basic`, `bare`, controlled-release publication, runnable import, and
   explicit immutable assignments retain their existing protections.
