import type { Descriptor } from "@/lib/audit";
import type { CodexHarnessPresentation } from "@/lib/auditCodex";

// EventSummary renders the trailing part of an event line: a Codex command as
// code, otherwise the Codex text or descriptor preview, each with its status.
export function EventSummary({
  d,
  codex,
}: {
  d: Descriptor;
  codex: CodexHarnessPresentation | undefined;
}) {
  if (codex?.type === "command") {
    return (
      <>
        <code className="min-w-0 flex-1 truncate text-muted-foreground" title={codex.text}>
          {codex.text}
        </code>
        <span className="shrink-0 text-muted-foreground">{codex.status}</span>
      </>
    );
  }
  return (
    <>
      <span className="min-w-0 flex-1 truncate text-muted-foreground">
        {codex?.text ?? d.preview}
      </span>
      {codex?.status && <span className="shrink-0 text-muted-foreground">{codex.status}</span>}
    </>
  );
}
