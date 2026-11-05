import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, type ReactNode } from "react";
import type { DisplayRow } from "@/lib/audit";
import { withSurface } from "@/storybook/decorators";
import { AuditRow } from "./AuditRow";

// AuditRow renders one chat-style transcript line — time · icon · label ·
// preview — with a semantic tone bar, expanding to the pretty-printed record.
// Fixtures follow AuditLogEvent / DisplayRow in src/lib/audit.ts.
const meta = {
  component: AuditRow,
  decorators: [withSurface("panel")],
  // The proxy-call rows read the API host (useApi).
  parameters: { tariboy: { daemon: true } },
} satisfies Meta<typeof AuditRow>;
export default meta;

type Story = StoryObj;

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

const assistantLine = (content: unknown[]) => hline({ type: "assistant", message: { content } });

const started = ev(1, "iteration_started", { trigger: "loop" }, "2026-09-11T18:22:41Z");
const launching = ev(
  2,
  "launching_harness",
  { harness: "claude", interactive: false },
  "2026-09-11T18:22:42Z",
);
const assistant = ev(
  3,
  "harness_output",
  assistantLine([
    { type: "text", text: "Running the desktop updater contract tests before editing." },
  ]),
  "2026-09-11T18:22:48Z",
  "harness",
);
const toolUse = ev(
  4,
  "harness_output",
  assistantLine([
    {
      type: "tool_use",
      name: "Bash",
      input: { command: "go test ./internal/commands/... -run Updater" },
    },
  ]),
  "2026-09-11T18:22:49Z",
  "harness",
);
const toolResult = ev(
  5,
  "harness_output",
  hline({
    type: "user",
    message: {
      content: [
        { type: "tool_result", content: [{ text: "ok  tariboy/internal/commands  1.284s" }] },
      ],
    },
  }),
  "2026-09-11T18:23:02Z",
  "harness",
);
const status = ev(
  6,
  "status",
  { message: "iteration productive · 3 files changed" },
  "2026-09-11T18:23:05Z",
);
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
  delta: [
    {
      role: "user",
      blocks: [{ type: "text" as const, text: "Goal TB-142 — ship the desktop updater contract." }],
    },
  ],
  response: {
    blocks: [
      { type: "text" as const, text: "Running the desktop updater contract tests before editing." },
    ],
    stop_reason: "end_turn",
  },
};

const noop = () => {};

const Line = ({
  row,
  open = false,
  scoped = false,
}: {
  row: DisplayRow;
  open?: boolean;
  scoped?: boolean;
}) => (
  <AuditRow
    row={row}
    open={open}
    onToggle={noop}
    {...(scoped ? { name: "builder", iteration: ITER } : {})}
  />
);

const eventRow = (key: number, event: ReturnType<typeof ev>): DisplayRow => ({
  kind: "event",
  key,
  event,
});

const thinkingRow = (key: number): DisplayRow => ({
  kind: "thinking",
  key,
  events: thinkingEvents,
  tokens: 2100,
});

const Sheet = ({ children }: { children: ReactNode }) => (
  <div className="w-[700px] rounded-md border border-border bg-muted p-2">{children}</div>
);

export const IterationTranscript: Story = {
  render: () => (
    <Sheet>
      <Line scoped row={eventRow(1, started)} />
      <Line scoped row={eventRow(2, launching)} />
      <Line row={thinkingRow(11)} />
      <Line scoped row={{ kind: "proxycall", key: -4, call, mode: "enrich" }} />
      <Line scoped row={eventRow(3, assistant)} />
      <Line scoped row={eventRow(4, toolUse)} />
      <Line scoped row={eventRow(5, toolResult)} />
      <Line scoped row={eventRow(6, status)} />
      <Line scoped row={eventRow(7, finished)} />
    </Sheet>
  ),
};

// Expanded, the row reveals a <details> holding the pretty-printed record. The
// effect opens that real <details> so the expanded state is visible at rest.
function ExpandedDemo() {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const d = ref.current?.querySelector("details");
    if (d) d.open = true;
  }, []);
  return (
    <div ref={ref} className="w-[700px] rounded-md border border-border bg-muted p-2">
      <Line scoped open row={eventRow(4, toolUse)} />
    </div>
  );
}

export const ExpandedToolCall: Story = { render: () => <ExpandedDemo /> };

// Consecutive reasoning events collapse into one dim marker with the summed
// token count; expanding it shows the raw events folded behind it.
export const ThinkingRun: Story = {
  render: () => (
    <Sheet>
      <Line row={thinkingRow(11)} />
      <Line open row={thinkingRow(21)} />
    </Sheet>
  ),
};

// The left bar is the semantic tone: success on a clean iteration_finished,
// amber on budget warnings, red on errors.
export const ToneAccents: Story = {
  render: () => (
    <Sheet>
      <Line row={eventRow(7, finished)} />
      <Line
        row={eventRow(
          8,
          ev(
            8,
            "budget_exceeded",
            { reason: "iteration budget 20 reached for builder" },
            "2026-09-11T18:24:10Z",
          ),
        )}
      />
      <Line
        row={eventRow(
          10,
          ev(
            10,
            "shim_error",
            { line: "dial tcp 10.0.4.11:7777: connect: connection refused" },
            "2026-09-11T18:24:31Z",
          ),
        )}
      />
      <Line
        row={eventRow(
          13,
          ev(13, "iteration_finished", { status: "harness_error" }, "2026-09-11T18:24:40Z"),
        )}
      />
    </Sheet>
  ),
};

// Codex harness items get their own presentation: command rows show the command
// and its status, and expand to the aggregated output.
export const CodexHarness: Story = {
  render: () => (
    <Sheet>
      <Line
        row={eventRow(
          31,
          codex(
            31,
            "2026-09-11T18:25:01Z",
            { type: "command_execution", command: "go build ./...", status: "running" },
            "item.started",
          ),
        )}
      />
      <Line
        row={eventRow(
          32,
          codex(32, "2026-09-11T18:25:04Z", {
            type: "agent_message",
            text: "The updater command now resolves after acknowledgement; packaging worker:v2.",
          }),
        )}
      />
      <Line
        row={eventRow(
          33,
          codex(33, "2026-09-11T18:25:09Z", {
            type: "file_change",
            changes: [
              { path: "desktop/src/updater.rs", kind: "modified" },
              { path: "desktop/tests/updater.rs", kind: "added" },
            ],
          }),
        )}
      />
      <Line
        open
        row={eventRow(
          34,
          codex(34, "2026-09-11T18:25:22Z", {
            type: "command_execution",
            command: "cargo test -p tariboy-desktop updater",
            exit_code: 0,
            aggregated_output:
              "running 9 tests\ntest updater::acknowledged ... ok\n\ntest result: ok. 9 passed; 0 failed",
          }),
        )}
      />
    </Sheet>
  ),
};
