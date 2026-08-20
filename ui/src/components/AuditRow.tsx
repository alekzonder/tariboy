import { codexHarnessPresentation, descriptor, hhmmss, fmtTokens, tone, pretty, isBoundary, TONE_BAR, type AuditEvent, type DisplayRow } from "@/lib/audit";
import { cn } from "@/lib/utils";
import { ProxyCallRow } from "@/components/ProxyCallRow";

// prettyEvents renders the collapsed thinking run as a JSON array so the marker
// still expands to the raw events behind it.
function prettyEvents(events: AuditEvent[]): string {
  return JSON.stringify(events.map((e) => ({ seq: e.seq, at: e.at, kind: e.kind, source: e.source })), null, 2);
}

// AuditRow renders one chat-style transcript line — time · icon · label ·
// preview — with a semantic tone bar, expanding to the pretty-printed record.
// Iteration boundaries are accented; thinking runs render as one dim marker.
export function AuditRow({
  row,
  open,
  onToggle,
  name,
  iteration,
}: {
  row: DisplayRow;
  open: boolean;
  onToggle: (key: number) => void;
  name?: string;
  iteration?: string;
}) {
  if (row.kind === "proxycall") {
    return <ProxyCallRow call={row.call} mode={row.mode} name={name ?? ""} iteration={iteration ?? ""} />;
  }

  if (row.kind === "thinking") {
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

  const e = row.event;
  const d = descriptor(e);
  const codex = codexHarnessPresentation(e);
  const boundary = isBoundary(e.kind);
  return (
    <div className={cn(boundary ? "border-l-4" : "border-l-2", TONE_BAR[tone(e.kind, e.data)])}>
      <button
        onClick={() => onToggle(row.key)}
        aria-expanded={open}
        aria-label={`${hhmmss(e.at)} ${codex?.label ?? d.label}${codex?.status ? `, ${codex.status}` : ""}. ${open ? "Collapse" : "Expand"} event details.`}
        className={cn(
          "flex w-full items-start gap-2 border-b py-1 pl-2 text-left font-mono text-xs hover:bg-accent last:border-0",
          boundary && "bg-muted/60 font-semibold",
        )}
      >
        <span className="shrink-0 text-muted-foreground">{hhmmss(e.at)}</span>
        <span className="shrink-0">{codex?.icon ?? d.icon}</span>
        <span className="shrink-0 font-semibold">{codex?.label ?? d.label}</span>
        {codex?.type === "command" ? (
          <>
            <code className="min-w-0 flex-1 truncate text-muted-foreground" title={codex.text}>{codex.text}</code>
            <span className="shrink-0 text-muted-foreground">{codex.status}</span>
          </>
        ) : (
          <>
            <span className="min-w-0 flex-1 truncate text-muted-foreground">{codex?.text ?? d.preview}</span>
            {codex?.status && <span className="shrink-0 text-muted-foreground">{codex.status}</span>}
          </>
        )}
      </button>
      {open && (
        <div className="bg-background/60 p-2 text-xs">
          {codex && (
            <div className="mb-2 space-y-1" aria-label={codex.type === "command" ? "Command details" : "Activity details"}>
              <p><span className="font-semibold">{codex.label}:</span> {codex.text}</p>
              {codex.status && <p><span className="font-semibold">Status:</span> {codex.status}</p>}
              {codex.output !== undefined && <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded border p-2"><span className="font-semibold">Output:</span>{"\n"}{codex.output}</pre>}
            </div>
          )}
          <details>
            <summary className="cursor-pointer">Raw event data</summary>
            <pre className="mt-1 overflow-x-auto whitespace-pre-wrap break-words">{pretty(e)}</pre>
          </details>
        </div>
      )}
    </div>
  );
}
