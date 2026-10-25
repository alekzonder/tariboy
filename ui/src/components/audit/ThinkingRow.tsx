import { fmtTokens, type AuditLogEvent, type DisplayRow } from "@/lib/audit";

// prettyEvents renders the collapsed thinking run as a JSON array so the marker
// still expands to the raw events behind it.
function prettyEvents(events: AuditLogEvent[]): string {
  return JSON.stringify(
    events.map((e) => ({ seq: e.seq, at: e.at, kind: e.kind, source: e.source })),
    null,
    2,
  );
}

// ThinkingRow renders a collapsed thinking run as one dim marker.
export function ThinkingRow({
  row,
  open,
  onToggle,
}: {
  row: Extract<DisplayRow, { kind: "thinking" }>;
  open: boolean;
  onToggle: (key: number) => void;
}) {
  return (
    <div className="border-l-2 border-l-transparent">
      <button
        onClick={() => onToggle(row.key)}
        className="flex w-full items-start gap-2 border-b py-1 pl-2 text-left font-mono text-xs italic text-muted-foreground/70 hover:bg-accent last:border-0"
      >
        <span className="shrink-0">🧠</span>
        <span>thinking…{row.tokens > 0 ? ` (${fmtTokens(row.tokens)} tokens)` : ""}</span>
      </button>
      {open && (
        <pre className="overflow-x-auto whitespace-pre-wrap break-words bg-background/60 p-2 text-xs">
          {prettyEvents(row.events)}
        </pre>
      )}
    </div>
  );
}
