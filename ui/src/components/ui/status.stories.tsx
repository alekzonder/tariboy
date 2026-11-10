import type { Meta, StoryObj } from "@storybook/react-vite";
import { PriorityTag, StatusDot, StatusPill } from "./status";

const meta = { component: StatusPill } satisfies Meta<typeof StatusPill>;
export default meta;

type Story = StoryObj;

const CARD = "overflow-hidden rounded-panel border border-border bg-card text-body";
const ROW = "flex h-8.5 items-center gap-2.5 px-3.5";
const KEY = "w-14 font-mono text-caption text-muted-foreground";
const TITLE = "min-w-0 flex-1 truncate";
const MONO_LABEL = "font-mono text-caption text-muted-foreground";

// ---- PriorityTag ----

// P0 and P1 are the only ranks that earn the danger fill.
const RANKS = ["P0", "P1", "P2", "P3", "P4"];

const PRIORITY_ROWS = [
  { key: "TB-142", title: "Ship the metal theme", priority: "P1" },
  { key: "TB-147", title: "Answer the packaging question", priority: "P2" },
  { key: "TB-151", title: "Remote provision smoke fails", priority: "P0" },
  { key: "TB-153", title: "Audit export column widths", priority: "P3" },
];

const FILL_GROUPS = [
  { ranks: ["P0", "P1"], note: "danger fill — the rank that changes what you do next" },
  { ranks: ["P2", "P3", "P4"], note: "neutral chip" },
];

export const Ranks: Story = {
  render: () => (
    <div className="flex items-center gap-2.5">
      {RANKS.map((rank) => (
        <PriorityTag key={rank} priority={rank} aria-label={`Priority ${rank}`} />
      ))}
    </div>
  ),
};

export const FillRule: Story = {
  render: () => (
    <div className="grid gap-2.5">
      {FILL_GROUPS.map((group) => (
        <div key={group.note} className="flex items-center gap-2.5">
          {group.ranks.map((rank) => (
            <PriorityTag key={rank} priority={rank} aria-label={`Priority ${rank}`} />
          ))}
          <span className="text-caption text-muted-foreground">{group.note}</span>
        </div>
      ))}
    </div>
  ),
};

// In place: the fixed-width priority column of the task table.
export const InTaskTable: Story = {
  render: () => (
    <div className={`${CARD} w-105`}>
      {PRIORITY_ROWS.map((row, i) => (
        <div key={row.key} className={`${ROW} ${i === 0 ? "" : "border-t border-border"}`}>
          <span className={KEY}>{row.key}</span>
          <span className={TITLE}>{row.title}</span>
          <span className="w-7.5 shrink-0">
            <PriorityTag priority={row.priority} aria-label={`Priority ${row.priority}`} />
          </span>
        </div>
      ))}
    </div>
  ),
};

// ---- StatusDot ----

const DOT_TONES = [
  { tone: "live" as const, meaning: "running — the only dot with a halo" },
  { tone: "attention" as const, meaning: "waiting on a person" },
  { tone: "danger" as const, meaning: "failed / out of budget" },
  { tone: "quiet" as const, meaning: "stopped, queued, done" },
  { tone: "faint" as const, meaning: "cancelled" },
];

const SIDEBAR_AGENTS = [
  { name: "builder", tone: "live" as const },
  { name: "reviewer", tone: "quiet" as const },
  { name: "packager", tone: "danger" as const },
  { name: "docs-bot", tone: "quiet" as const },
];

export const StatusDotTones: Story = {
  render: () => (
    <div className="grid gap-2.75">
      {DOT_TONES.map((t) => (
        <div key={t.tone} className="flex items-center gap-3">
          <span className="grid w-3.5 place-items-center">
            <StatusDot tone={t.tone} />
          </span>
          <span className={`w-17 ${MONO_LABEL}`}>{t.tone}</span>
          <span className="text-ui">{t.meaning}</span>
        </div>
      ))}
    </div>
  ),
};

// 7px in a sidebar row, 8px in the agent header — the only two sizes.
export const StatusDotSizes: Story = {
  render: () => (
    <div className="flex items-center gap-5.5">
      {([7, 8] as const).map((size) => (
        <div key={size} className="flex items-center gap-2">
          <span className="grid w-3.5 place-items-center">
            <StatusDot tone="live" size={size} />
          </span>
          <span className="text-caption text-muted-foreground">{size}px</span>
        </div>
      ))}
    </div>
  ),
};

// In place: the leading marker of a sidebar agent row.
export const InSidebar: Story = {
  render: () => (
    <div className="w-60 rounded-panel border border-sidebar-border bg-sidebar p-1.5">
      {SIDEBAR_AGENTS.map((agent) => (
        <div
          key={agent.name}
          className="flex h-7.5 items-center gap-2.25 px-2.5 text-body text-sidebar-foreground"
        >
          <StatusDot tone={agent.tone} />
          <span className="font-medium">{agent.name}</span>
        </div>
      ))}
    </div>
  ),
};

// ---- StatusPill ----

const PILL_TONES = [
  { tone: "live" as const, label: "In progress", meaning: "running / in progress" },
  { tone: "attention" as const, label: "Wait customer", meaning: "needs a person" },
  { tone: "danger" as const, label: "Failed", meaning: "failed / out of budget" },
  { tone: "quiet" as const, label: "Open", meaning: "queued, stopped, open, done" },
  { tone: "faint" as const, label: "Cancelled", meaning: "cancelled" },
];

const PILL_ROWS = [
  { key: "TB-142", title: "Ship the metal theme", tone: "live" as const, status: "In progress" },
  {
    key: "TB-147",
    title: "Answer the packaging question",
    tone: "attention" as const,
    status: "Wait customer",
  },
  {
    key: "TB-151",
    title: "Remote provision smoke fails",
    tone: "danger" as const,
    status: "Failed",
  },
  { key: "TB-153", title: "Audit export column widths", tone: "quiet" as const, status: "Open" },
];

export const Tones: Story = {
  render: () => (
    <div className="grid gap-2.25">
      {PILL_TONES.map((t) => (
        <div key={t.tone} className="flex items-center gap-3">
          <span className={`w-18.5 ${MONO_LABEL}`}>{t.tone}</span>
          <StatusPill tone={t.tone}>{t.label}</StatusPill>
          <span className="text-caption text-muted-foreground">{t.meaning}</span>
        </div>
      ))}
    </div>
  ),
};

// quiet="chip" keeps the tone and only adds the neutral background.
export const QuietAsChip: Story = {
  render: () => (
    <div className="grid gap-2.25">
      {(["quiet", "faint"] as const).map((tone) => (
        <div key={tone} className="flex items-center gap-3">
          <span className={`w-18.5 ${MONO_LABEL}`}>{tone}</span>
          <StatusPill tone={tone}>text</StatusPill>
          <StatusPill tone={tone} quiet="chip">
            chip
          </StatusPill>
        </div>
      ))}
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="grid gap-2.25">
      {(["sm", "md"] as const).map((size) => (
        <div key={size} className="flex items-center gap-2.5">
          <span className={`w-18.5 ${MONO_LABEL}`}>{size}</span>
          <StatusPill tone="live" size={size}>
            running
          </StatusPill>
          <StatusPill tone="danger" size={size}>
            no budget
          </StatusPill>
          <StatusPill tone="quiet" size={size} quiet="chip">
            stopped
          </StatusPill>
        </div>
      ))}
    </div>
  ),
};

// In place: the status column of the task table.
export const InTaskRow: Story = {
  render: () => (
    <div className={`${CARD} w-115`}>
      {PILL_ROWS.map((row, i) => (
        <div key={row.key} className={`${ROW} ${i === 0 ? "" : "border-t border-border"}`}>
          <span className={KEY}>{row.key}</span>
          <span className={TITLE}>{row.title}</span>
          <span className="w-27.5 shrink-0">
            <StatusPill tone={row.tone}>{row.status}</StatusPill>
          </span>
        </div>
      ))}
    </div>
  ),
};
