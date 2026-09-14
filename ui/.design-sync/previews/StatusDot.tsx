import { StatusDot } from "tariboy-ui";

// Only a live dot carries the halo (ring-status-running/18); a quiet one is
// the flat stopped grey, which is deliberately a different token from muted
// text. See TONE_DOT in src/lib/statusTone.ts.
const TONES = [
  { tone: "live" as const, meaning: "running — the only dot with a halo" },
  { tone: "attention" as const, meaning: "waiting on a person" },
  { tone: "danger" as const, meaning: "failed / out of budget" },
  { tone: "quiet" as const, meaning: "stopped, queued, done" },
  { tone: "faint" as const, meaning: "cancelled" },
];

export const Tones = () => (
  <div style={{ display: "grid", gap: 11 }}>
    {TONES.map((t) => (
      <div key={t.tone} style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span style={{ display: "grid", placeItems: "center", width: 14 }}>
          <StatusDot tone={t.tone} />
        </span>
        <span
          style={{
            width: 68,
            fontSize: 11,
            fontFamily: "var(--font-mono)",
            color: "var(--muted-foreground)",
          }}
        >
          {t.tone}
        </span>
        <span style={{ fontSize: 12 }}>{t.meaning}</span>
      </div>
    ))}
  </div>
);

// 7px in a sidebar row, 8px in the agent header — the only two sizes.
export const Sizes = () => (
  <div style={{ display: "flex", alignItems: "center", gap: 22 }}>
    {([7, 8] as const).map((size) => (
      <div key={size} style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <span style={{ display: "grid", placeItems: "center", width: 14 }}>
          <StatusDot tone="live" size={size} />
        </span>
        <span style={{ fontSize: 11.5, color: "var(--muted-foreground)" }}>
          {size}px
        </span>
      </div>
    ))}
  </div>
);

// In place: the leading marker of a sidebar agent row.
export const InSidebar = () => (
  <div
    style={{
      width: 240,
      padding: 6,
      borderRadius: "var(--panel-radius)",
      background: "var(--sidebar)",
      border: "1px solid var(--sidebar-border)",
    }}
  >
    {[
      { name: "builder", tone: "live" as const },
      { name: "reviewer", tone: "quiet" as const },
      { name: "packager", tone: "danger" as const },
      { name: "docs-bot", tone: "quiet" as const },
    ].map((agent) => (
      <div
        key={agent.name}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 9,
          height: 30,
          padding: "0 10px",
          fontSize: 13,
          color: "var(--sidebar-foreground)",
        }}
      >
        <StatusDot tone={agent.tone} />
        <span style={{ fontWeight: 500 }}>{agent.name}</span>
      </div>
    ))}
  </div>
);
