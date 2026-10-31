---
title: Autopilot
description: Run bounded, observable iterations on timers and durable message triggers.
sidebar:
  label: Autopilot
  icon: repeat
---

Autopilot is the daemon-controlled iteration loop of a started agent. An agent
has one on/off switch: **Start** turns the whole agent on (iterations, messages,
Goal, and the interactive Terminal), and **Stop** turns all of it off. There is
no separate Autopilot switch.

Agents from the terminal-only `bare:latest` image never run autonomous
iterations. To keep a non-bare agent from iterating on a timer, set its
interval to 0. In **Settings → Autopilot**, select **Only when given tasks or
messages** and click **Save changes**. A separate schedule under **Advanced →
Schedule and timeouts** can still start runs. Set **Schedule** to **None**
there to run only for messages, including Goal tasks.

## Trigger model

While the agent is started, an iteration can start from:

- a configured timer interval or schedule; or
- an open letter in the agent's inbox, including a `task.goal` letter.

Letters are durable. Sending a letter to the agent stores it in its inbox and
nudges the loop. The runner includes the open letters in the next prompt. A
request stays open until the agent replies to it; a notice closes when the
iteration finishes normally. Harness errors, timeouts, and kills leave both
open for bounded redelivery.

Stopping the agent prevents new scheduled and message-triggered iterations. It
does not delete open letters.

A stop policy (`on_timeout=stop` or `on_error=stop`) halts the loop with an
error reason. The agent stays started and shows the `error` state. Start it
again to clear the reason and resume the loop.

## Configure conservatively

Start with:

- an image prompt that defines one coherent unit of work;
- a long enough interval to inspect outcomes;
- a soft timeout that fits normal work;
- a hard timeout that bounds the worst case;
- a low cost budget;
- no external plugins sending mail until the timer path is understood.

Set the interval and soft timeout in minutes under **Settings → Autopilot**.
**Also every** sets a timer interval. **Stop a run after** sets the soft
timeout, with `0` for no limit. Open **Advanced → Schedule and timeouts** for
the schedule, hard timeout in seconds, and timeout and error policies.
Set daily and monthly USD limits under **Spending limit**. Hourly and weekly
limits are under **Advanced → Hour and week budgets (USD)**.
Click **Save changes** at the top to apply the draft.
During agent creation, the interval and timeouts are under **Advanced →
Autopilot**.

Activity shows trigger, iteration state, result, deadlines, audit, usage,
and cost. For Claude and
Codex harnesses, the audit is presented as a readable timeline of messages,
reasoning, commands, skills, tool calls, and results; model and token metadata
remains available on each AI call. A running soft deadline may be extended only
before timeout enforcement starts.

## Controls

- **Start / Stop** (agent header) — turn the agent on or off. From the CLI use
  `tariboy agent start NAME` and `tariboy agent stop NAME`.
- **Run once…** (the `⋯` menu of the agent header, shown while the agent is
  enabled) — start one manual iteration immediately. It is available for both
  interactive and non-interactive agents; optional one-shot text, entered in
  its dialog, is appended only to that iteration's assembled prompt.
- **Force-stop current run** (the `⋯` menu of the agent header) — after you
  confirm, immediately stop the current iteration or session; use when work is
  unsafe or stuck.
- **Budget** — reject AI calls once the configured spend limit is reached.
- **Audit** (Activity → **Runs log**) — inspect each proxied AI request and
  tool timeline. Copy readable
  Markdown or export a ZIP for the selected iteration or all retained
  iterations. Exports contain sensitive agent data; review the warning before
  downloading them.

After a force-stop, verify the Activity outcome and the agent's open letters before
starting the agent again. A killed or failed iteration leaves its letters
open for redelivery.

## Event-driven progression

Mail is the layer behind message triggers. It lets agents, plugins, schedules
and scripts wake work without polling. Begin with one sender writing to one
agent, then add groups after the single-agent letter lifecycle is understood.

Read an agent's inbox, sent letters and archive, with its dead letters, with
`tariboy mail inbox --as agent:NAME`, `sent` and `archive`.

See [Mailbox architecture](/docs/architecture/messaging) and the [Mail
reference](/docs/reference/mail) for the complete contract.
