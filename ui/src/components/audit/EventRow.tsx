import {
  descriptor,
  hhmmss,
  tone,
  pretty,
  isBoundary,
  TONE_BAR,
  type AuditLogEvent,
  type Descriptor,
  type DisplayRow,
} from "@/lib/audit";
import { codexHarnessPresentation, type CodexHarnessPresentation } from "@/lib/auditCodex";
import { cn } from "@/lib/utils";
import { EventSummary } from "./EventSummary";
import { CodexDetails } from "./CodexDetails";

// eventAriaLabel names the row toggle: time, label, optional status and the
// expand/collapse action.
function eventAriaLabel(
  e: AuditLogEvent,
  d: Descriptor,
  codex: CodexHarnessPresentation | undefined,
  open: boolean,
): string {
  const status = codex?.status ? `, ${codex.status}` : "";
  return `${hhmmss(e.at)} ${codex?.label ?? d.label}${status}. ${open ? "Collapse" : "Expand"} event details.`;
}

// EventRow renders one chat-style event line with a semantic tone bar,
// expanding to the Codex activity details and the pretty-printed record.
export function EventRow({
  row,
  open,
  onToggle,
}: {
  row: Extract<DisplayRow, { kind: "event" }>;
  open: boolean;
  onToggle: (key: number) => void;
}) {
  const e = row.event;
  const d = descriptor(e);
  const codex = codexHarnessPresentation(e);
  const boundary = isBoundary(e.kind);
  return (
    <div className={cn(boundary ? "border-l-4" : "border-l-2", TONE_BAR[tone(e.kind, e.data)])}>
      <button
        onClick={() => onToggle(row.key)}
        aria-expanded={open}
        aria-label={eventAriaLabel(e, d, codex, open)}
        className={cn(
          "flex w-full items-start gap-2 border-b py-1 pl-2 text-left font-mono text-xs hover:bg-accent last:border-0",
          boundary && "bg-muted/60 font-semibold",
        )}
      >
        <span className="shrink-0 text-muted-foreground">{hhmmss(e.at)}</span>
        <span className="shrink-0">{codex?.icon ?? d.icon}</span>
        <span className="shrink-0 font-semibold">{codex?.label ?? d.label}</span>
        <EventSummary d={d} codex={codex} />
      </button>
      {open && (
        <div className="bg-background/60 p-2 text-xs">
          {codex && <CodexDetails codex={codex} />}
          <details>
            <summary className="cursor-pointer">Raw event data</summary>
            <pre className="mt-1 overflow-x-auto whitespace-pre-wrap break-words">{pretty(e)}</pre>
          </details>
        </div>
      )}
    </div>
  );
}
