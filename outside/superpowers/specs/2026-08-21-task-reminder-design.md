# Task Reminder Design

## Goal

Allow an operator to opt in to reminders for agents that have assigned open
Native Tasks but have remained idle. A reminder must reach the agent through
the ordinary channel bus and inbox delivery path, so the existing loop controls,
delivery durability, and wake coalescing remain authoritative.

## Configuration

The daemon configuration key `task_reminder` stores one JSON object:

```json
{"enabled":false,"idle_threshold_s":300}
```

- `enabled` defaults to `false` when the key is absent.
- `idle_threshold_s` defaults to `300`, is persisted in seconds, and must be a
  positive integral number.
- The setting applies to every Autopilot cadence, including `interval_s=0`.
- Invalid stored values are treated as disabled and logged as a stable
  configuration error. They never create a reminder.

The existing daemon configuration API remains the configuration group. Its
setter validates `task_reminder` before persistence and returns the normalized
JSON value; unrelated keys retain their current string behavior.

## Eligibility and delivery

One daemon-owned reconciler runs at startup and on a bounded periodic cadence.
For each scan it obtains the currently configured reminder policy and finds
agents that satisfy all of these conditions:

1. The policy is enabled.
2. The agent is enabled and its Autopilot loop is enabled.
3. The agent has at least one Native Task assigned to its authenticated agent
   principal whose status is still open (not `done` or otherwise terminal).
4. The agent has been idle for at least `idle_threshold_s`, based on the
   latest durable iteration completion/activity boundary.
5. The same open-work generation has not already received a reminder since
   that boundary.

For each eligible agent, the reconciler publishes a normal `task.reminder`
message to `agent:<name>:inbox` with source `tasks`, reason
`assigned-work-idle`, the threshold in seconds, and the stable task-key list.
It does not call the loop engine directly, start an iteration directly, or
change Native Task status. The bus creates the normal protected-inbox delivery,
then its existing publish hook invokes `WakeMessage`; the loop still requires
an enabled agent and pending delivery before beginning an iteration.

## Dedupe, activity, and recovery

Reminder state is persisted as a per-agent fingerprint of the assigned-open
task keys plus the activity boundary used for eligibility. Publishing and
recording that fingerprint occur atomically enough to prevent repeated scans
from flooding an agent. A changed assignment set or a newer completed
iteration creates a new generation and permits one later reminder after the
threshold. A daemon restart loads the same durable boundary and does not resend
the same reminder merely because the process restarted.

The reconciler uses the same SQLite source of truth as Native Tasks. It is
safe if a task changes between selection and delivery: the subsequent agent
iteration claims work from the authoritative task store. Reconciler failures
are logged and retried on the next cadence; one failed agent cannot prevent
other candidates from being checked.

## UI and documentation

The Configuration group exposes an off-by-default Task reminders section with
a toggle and an idle threshold input in seconds. It reads and writes the
normalized `task_reminder` daemon configuration. Documentation states the
defaults, seconds unit, eligible-agent conditions, and normal inbox/channel
delivery behavior.

## Test strategy

Focused tests cover configuration defaults, validation, and round-tripping;
candidate selection across zero and positive intervals; no send before the
idle threshold; one durable inbox reminder after it; suppression for the same
generation; reset on new activity or a changed assignment set; disabled and
invalid configuration; and startup/shutdown lifecycle. UI tests cover the
default-off form, invalid threshold feedback, and normalized saves. The final
suite uses isolated daemon state only.
