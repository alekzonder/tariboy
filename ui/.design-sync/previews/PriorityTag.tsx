import { PriorityTag } from "tariboy-ui";

// priorityTone(): P0 and P1 are the only ranks that earn the danger fill;
// everything else is the neutral chip. The tag is mono + tabular-nums so a
// column of them stays aligned.
const RANKS = ["P0", "P1", "P2", "P3", "P4"];

export const Ranks = () => (
  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
    {RANKS.map((rank) => (
      <PriorityTag key={rank} priority={rank} aria-label={`Priority ${rank}`} />
    ))}
  </div>
);

export const FillRule = () => (
  <div style={{ display: "grid", gap: 10 }}>
    {[
      { ranks: ["P0", "P1"], note: "danger fill — the rank that changes what you do next" },
      { ranks: ["P2", "P3", "P4"], note: "neutral chip" },
    ].map((group) => (
      <div key={group.note} style={{ display: "flex", alignItems: "center", gap: 10 }}>
        {group.ranks.map((rank) => (
          <PriorityTag key={rank} priority={rank} aria-label={`Priority ${rank}`} />
        ))}
        <span style={{ fontSize: 11.5, color: "var(--muted-foreground)" }}>{group.note}</span>
      </div>
    ))}
  </div>
);

// In place: the fixed-width priority column of the task table, which is what
// the tag is sized and aligned for.
export const InTaskTable = () => (
  <div
    style={{
      width: 420,
      border: "1px solid var(--border)",
      borderRadius: "var(--panel-radius)",
      background: "var(--card)",
      overflow: "hidden",
      fontSize: 13,
    }}
  >
    {[
      { key: "TB-142", title: "Ship the metal theme", priority: "P1" },
      { key: "TB-147", title: "Answer the packaging question", priority: "P2" },
      { key: "TB-151", title: "Remote provision smoke fails", priority: "P0" },
      { key: "TB-153", title: "Audit export column widths", priority: "P3" },
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
        <span style={{ width: 30, flexShrink: 0 }}>
          <PriorityTag priority={row.priority} aria-label={`Priority ${row.priority}`} />
        </span>
      </div>
    ))}
  </div>
);
