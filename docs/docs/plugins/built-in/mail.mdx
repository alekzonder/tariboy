---
title: mail
description: Read the inbox, send letters, and reply to them with tmail.
sidebar:
  label: mail
  icon: mail
---

`mail` connects an agent to its mailbox. It is a historical core capability,
named `messages` before the mailbox; schema-v2 images must declare it
explicitly. The image contract still accepts the plugin `messages` with its
`messages` skill as an alias of `mail`, so images built before the mailbox keep
provisioning and keep rendering their letters.

## Capability surface

The agent uses `tmail`, which agent provisioning puts on its `PATH`. In an
agent, `tmail` talks to `POST /tools/mail/<action>` on the identity-bound
socket, and the sender is always the calling agent.

| Operation | Commands |
| --- | --- |
| Read | `tmail inbox [--all]`, `tmail sent`, `tmail archive`, `tmail show ID` |
| Send | `tmail send --to P --subject S --text T [--deadline D] [--notice]` |
| Answer | `tmail reply ID TEXT [--ask]` |
| Recovery | `tmail requeue ID` for a dead letter in the archive |

An open letter in the inbox wakes a started agent and is rendered into the
next iteration in a bounded batch. A request stays open until the agent replies
to it; a notice closes when an iteration that rendered it finishes
successfully. A `task.question` letter is answered on its task with `ttasks
comment`, not with `tmail reply`.

`tmail send --deadline` sends a timed request. If it is still open at the
deadline, the agent receives a `deadline` notice from `system:schedule`.

## Prompt integration

```yaml Tariboyfile.yaml
plugins:
  - name: mail
skills:
  - dir: ../../skills/mail
prompts:
  - runtime: messages
```

The `messages` runtime marker is a compatibility marker: the open letters,
requests first and notices after them, with the standing instruction to answer
with `tmail reply`, and the agent's timed requests that have no answer yet
appear in the platform-owned Task Processing Order block. The older
`awaiting-replies` marker remains accepted for existing built images. Both
markers require the `mail` capability or its `messages` alias.

## Failure and recovery

A letter rendered five times without closing is dead-lettered and moves to the
agent's archive, where the agent can requeue it. A letter sent while the
agent's open inbox is at `messages_max_queue` is stored dead with reason
`queue_limit`. The old `scripts/messages.sh` launcher of the `messages` skill
fails with `unknown route` against the mailbox; the prompt names `tmail reply`
in any case.

## Related reference

- [Mailbox architecture](/docs/architecture/messaging)
- [Mail reference](/docs/reference/mail)
