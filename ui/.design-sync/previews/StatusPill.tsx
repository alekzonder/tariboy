import { StatusPill } from "tariboy-ui";

// The five tones of src/lib/statusTone.ts. A fill is reserved for what is
// ALIVE (live) and what NEEDS A PERSON (attention, danger); quiet and faint
// are text, not chips.
const TONES = [
  { tone: "live" as const, label: "In progress", meaning: "running / in progress" },
  { tone: "attention" as const, label: "Wait customer", meaning: "needs a person" },
  { tone: "danger" as const, label: "Failed", meaning: "failed / out of budget" },
  { tone: "quiet" as const, label: "Open", meaning: "queued, stopped, open, done" },
  { tone: "faint" as const, label: "Cancelled", meaning: "cancelled" },
];

const label: React.CSSProperties = {
  width: 74,
  fontSize: 11,
  fontFamily: "var(--font-mono)",
  color: "var(--muted-foreground)",
};

export const Tones = () => (
  <div style={{ display: "grid", gap: 9 }}>
    {TONES.map((t) => (
      <div key={t.tone} style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span style={label}>{t.tone}</span>
        <StatusPill tone={t.tone}>{t.label}</StatusPill>
        <span style={{ fontSize: 11.5, color: "var(--muted-foreground)" }}>{t.meaning}</span>
      </div>
    ))}
  </div>
);

// quiet="chip" is what a layout uses when the pill shape has to hold a
// column — the tone is unchanged, only the neutral background appears.
export const QuietAsChip = () => (
  <div style={{ display: "grid", gap: 9 }}>
    {(["quiet", "faint"] as const).map((tone) => (
      <div key={tone} style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span style={label}>{tone}</span>
        <StatusPill tone={tone}>text</StatusPill>
        <StatusPill tone={tone} quiet="chip">
          chip
        </StatusPill>
      </div>
    ))}
  </div>
);

// sm is the 17px chip of sidebar rows and priorities; md the 20px pill of
// task rows and the agent header.
export const Sizes = () => (
  <div style={{ display: "grid", gap: 9 }}>
    {(["sm", "md"] as const).map((size) => (
      <div key={size} style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <span style={label}>{size}</span>
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
);

// In place: the status column of the task table.
export const InTaskRow = () => (
  <div
    style={{
      width: 460,
      border: "1px solid var(--border)",
      borderRadius: "var(--panel-radius)",
      background: "var(--card)",
      overflow: "hidden",
      fontSize: 13,
    }}
  >
    {[
      { key: "TB-142", title: "Ship the metal theme", tone: "live" as const, status: "In progress" },
      { key: "TB-147", title: "Answer the packaging question", tone: "attention" as const, status: "Wait customer" },
      { key: "TB-151", title: "Remote provision smoke fails", tone: "danger" as const, status: "Failed" },
      { key: "TB-153", title: "Audit export column widths", tone: "quiet" as const, status: "Open" },
    ].map((row, i) => (
      <div
        key={row.key}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 10,
          padding: "0 14px",
          height: 34,
          borderTop: i === 0 ? "none" : "1px solid var(--border)",
        }}
      >
        <span
          style={{
            width: 56,
            fontFamily: "var(--font-mono)",
            fontSize: 11.5,
            color: "var(--muted-foreground)",
          }}
        >
          {row.key}
        </span>
        <span style={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {row.title}
        </span>
        <span style={{ width: 110, flexShrink: 0 }}>
          <StatusPill tone={row.tone}>{row.status}</StatusPill>
        </span>
      </div>
    ))}
  </div>
);
