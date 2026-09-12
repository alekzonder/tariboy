import { useEffect, useRef } from "react";
import { AuditRow } from "tariboy-ui";

// AuditRow renders one chat-style transcript line — time · icon · label ·
// preview — with a semantic tone bar, expanding to the pretty-printed record.
// Fixtures follow AuditEvent / DisplayRow in src/lib/audit.ts: `event` rows,
// the collapsed `thinking` run, and `proxycall` rows (which delegate to
// ProxyCallRow). Iteration boundaries get the accented, bolder row.

const ITER = "4f2a91c7-2b60-4f1e-8f0a-91d2c7e51a33";

const ev = (seq: number, kind: string, data: unknown, at: string, source = "shim") => ({
  seq,
  kind,
  source,
  data: JSON.stringify(data),
  at,
  iteration_id: ITER,
});

// harness_output carries the harness's stream-json as a JSON string in data.line.
const hline = (inner: unknown) => ({ line: JSON.stringify(inner) });

const started = ev(1, "iteration_started", { trigger: "loop" }, "2026-09-11T18:22:41Z");
const launching = ev(2, "launching_harness", { harness: "claude", interactive: false }, "2026-09-11T18:22:42Z");
const assistant = ev(
  3,
  "harness_output",
  hline({
    type: "assistant",
    message: { content: [{ type: "text", text: "Running the desktop updater contract tests before editing." }] },
  }),
  "2026-09-11T18:22:48Z",
  "harness",
);
const toolUse = ev(
  4,
  "harness_output",
  hline({
    type: "assistant",
    message: {
      content: [
        { type: "tool_use", name: "Bash", input: { command: "go test ./internal/commands/... -run Updater" } },
      ],
    },
  }),
  "2026-09-11T18:22:49Z",
  "harness",
);
const toolResult = ev(
  5,
  "harness_output",
  hline({
    type: "user",
    message: { content: [{ type: "tool_result", content: [{ text: "ok  tariboy/internal/commands  1.284s" }] }] },
  }),
  "2026-09-11T18:23:02Z",
  "harness",
);
const status = ev(6, "status", { message: "iteration productive · 3 files changed" }, "2026-09-11T18:23:05Z");
const finished = ev(7, "iteration_finished", { status: "ok" }, "2026-09-11T18:23:07Z");

const thinkingEvents = [
  ev(
    11,
    "harness_output",
    hline({ type: "system", subtype: "thinking_tokens", estimated_tokens: 1400 }),
    "2026-09-11T18:22:44Z",
    "harness",
  ),
  ev(
    12,
    "harness_output",
    hline({ type: "system", subtype: "thinking_tokens", estimated_tokens: 700 }),
    "2026-09-11T18:22:45Z",
    "harness",
  ),
];

const codex = (seq: number, at: string, item: Record<string, unknown>, outer = "item.completed") =>
  ev(seq, "harness_output", hline({ type: outer, item }), at, "harness");

const call = {
  seq: 3,
  ts: "2026-09-11T18:22:47Z",
  provider: "anthropic",
  model: "claude-opus-5",
  usage: { input: 18422, output: 1163, cache_read: 96000, cache_write: 4200 },
  cost_usd: 0.42,
  latency_ms: 8140,
  status: "ok",
  instructions: "You are the builder agent for the tariboy repository.",
  instructions_changed: false,
  delta: [{ role: "user", blocks: [{ type: "text" as const, text: "Goal TB-142 — ship the desktop updater contract." }] }],
  response: {
    blocks: [{ type: "text" as const, text: "Running the desktop updater contract tests before editing." }],
    stop_reason: "end_turn",
  },
};

const sheet = {
  width: 700,
  borderRadius: 6,
  border: "1px solid var(--border)",
  background: "var(--muted)",
  padding: 8,
} as const;

const noop = () => {};

export const IterationTranscript = () => (
  <div style={sheet}>
    <AuditRow row={{ kind: "event", key: 1, event: started }} open={false} onToggle={noop} name="builder" iteration={ITER} />
    <AuditRow row={{ kind: "event", key: 2, event: launching }} open={false} onToggle={noop} name="builder" iteration={ITER} />
    <AuditRow row={{ kind: "thinking", key: 11, events: thinkingEvents, tokens: 2100 }} open={false} onToggle={noop} />
    <AuditRow row={{ kind: "proxycall", key: -4, call, mode: "enrich" }} open={false} onToggle={noop} name="builder" iteration={ITER} />
    <AuditRow row={{ kind: "event", key: 3, event: assistant }} open={false} onToggle={noop} name="builder" iteration={ITER} />
    <AuditRow row={{ kind: "event", key: 4, event: toolUse }} open={false} onToggle={noop} name="builder" iteration={ITER} />
    <AuditRow row={{ kind: "event", key: 5, event: toolResult }} open={false} onToggle={noop} name="builder" iteration={ITER} />
    <AuditRow row={{ kind: "event", key: 6, event: status }} open={false} onToggle={noop} name="builder" iteration={ITER} />
    <AuditRow row={{ kind: "event", key: 7, event: finished }} open={false} onToggle={noop} name="builder" iteration={ITER} />
  </div>
);

// Expanded, the row reveals a <details> holding the pretty-printed record with
// data parsed back into nested JSON. The effect opens that real <details> so the
// expanded state is visible at rest — nothing about the row is stubbed.
export const ExpandedToolCall = () => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const d = ref.current?.querySelector("details");
    if (d) d.open = true;
  }, []);
  return (
    <div ref={ref} style={sheet}>
      <AuditRow row={{ kind: "event", key: 4, event: toolUse }} open onToggle={noop} name="builder" iteration={ITER} />
    </div>
  );
};

// Consecutive reasoning events collapse into one dim marker with the summed
// token count; expanding it shows the raw events folded behind it.
export const ThinkingRun = () => (
  <div style={sheet}>
    <AuditRow row={{ kind: "thinking", key: 11, events: thinkingEvents, tokens: 2100 }} open={false} onToggle={noop} />
    <AuditRow row={{ kind: "thinking", key: 21, events: thinkingEvents, tokens: 2100 }} open onToggle={noop} />
  </div>
);

// The left bar is the semantic tone: success on a clean iteration_finished,
// amber on budget/rate-limit warnings, red on errors.
export const ToneAccents = () => (
  <div style={sheet}>
    <AuditRow row={{ kind: "event", key: 7, event: finished }} open={false} onToggle={noop} />
    <AuditRow
      row={{
        kind: "event",
        key: 8,
        event: ev(8, "budget_exceeded", { reason: "iteration budget 20 reached for builder" }, "2026-09-11T18:24:10Z"),
      }}
      open={false}
      onToggle={noop}
    />
    <AuditRow
      row={{
        kind: "event",
        key: 9,
        event: ev(9, "rate_limited", { message: "provider rate limit · retrying in 30s" }, "2026-09-11T18:24:20Z"),
      }}
      open={false}
      onToggle={noop}
    />
    <AuditRow
      row={{
        kind: "event",
        key: 10,
        event: ev(10, "shim_error", { line: "dial tcp 10.0.4.11:7777: connect: connection refused" }, "2026-09-11T18:24:31Z"),
      }}
      open={false}
      onToggle={noop}
    />
    <AuditRow
      row={{
        kind: "event",
        key: 13,
        event: ev(13, "iteration_finished", { status: "harness_error" }, "2026-09-11T18:24:40Z"),
      }}
      open={false}
      onToggle={noop}
    />
  </div>
);

// Codex harness items get their own presentation: command rows show the command
// and its status, and expand to the aggregated output instead of a protocol
// envelope.
export const CodexHarness = () => (
  <div style={sheet}>
    <AuditRow
      row={{
        kind: "event",
        key: 31,
        event: codex(31, "2026-09-11T18:25:01Z", { type: "command_execution", command: "go build ./...", status: "running" }, "item.started"),
      }}
      open={false}
      onToggle={noop}
    />
    <AuditRow
      row={{
        kind: "event",
        key: 32,
        event: codex(32, "2026-09-11T18:25:04Z", {
          type: "agent_message",
          text: "The updater command now resolves after acknowledgement; packaging worker:v2.",
        }),
      }}
      open={false}
      onToggle={noop}
    />
    <AuditRow
      row={{
        kind: "event",
        key: 33,
        event: codex(33, "2026-09-11T18:25:09Z", {
          type: "file_change",
          changes: [{ path: "desktop/src/updater.rs", kind: "modified" }, { path: "desktop/tests/updater.rs", kind: "added" }],
        }),
      }}
      open={false}
      onToggle={noop}
    />
    <AuditRow
      row={{
        kind: "event",
        key: 34,
        event: codex(34, "2026-09-11T18:25:22Z", {
          type: "command_execution",
          command: "cargo test -p tariboy-desktop updater",
          exit_code: 0,
          aggregated_output: "running 9 tests\ntest updater::acknowledged ... ok\n\ntest result: ok. 9 passed; 0 failed",
        }),
      }}
      open
      onToggle={noop}
    />
  </div>
);
