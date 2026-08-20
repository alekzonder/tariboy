# Tariboy internal alpha: 90-second demo

## Before the clock

Use an already connected disposable Linux x86_64 host, a built `reviewer:latest`
image, and an agent named `review-demo`. Keep Activity empty enough that the new
iteration is obvious.

## Script

**0:00–0:12 — Frame**

“Tariboy is the control plane for autonomous coding agents. It starts as one
UI for terminal sessions across hosts, then adds reusable images, bounded
Autopilot, events, usage, and audit.”

Show the host selector and Agents list.

**0:12–0:28 — Image to agent**

Open Images, show `reviewer:latest`, then open the agent creation dialog.

“An image captures harness, model, prompt, and capabilities. An agent is a
durable workspace. Interactive and Autopilot are independent.”

**0:28–0:43 — Interactive**

Open `review-demo` → Console and show the live harness terminal.

“This is the familiar entry point. `bare:latest` is available when I want only
an ordinary instructions-free terminal.”

**0:43–1:03 — Autopilot and event**

Open Autopilot, point to the enabled state and bounded timeout. Trigger one
prepared message.

“Autopilot can wake on a timer or durable message. The daemon owns delivery and
bounded iterations; closing the terminal does not lose the work.”

**1:03–1:20 — Control and evidence**

Open Activity and show trigger, current iteration, usage, cost, and audit.

“I can see why it ran, what it is doing, what it cost, and what it called. Pause
prevents new iterations; Kill stops current unsafe work.”

**1:20–1:30 — Direction**

Show Agents, Images, Settings navigation.

“The alpha proves Image → Agent → Interactive → Autopilot. Next, the same
control plane makes events and coordinated agent teams approachable without
changing the operator experience.”

## Demo recovery

If remote health is not Ready, switch to Local rather than debugging live. If
the prepared event has already been consumed, show the previous Activity row
and explain its trigger. Never use a production host or real customer task for
the demo.
