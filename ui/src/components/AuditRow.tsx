import type { DisplayRow } from "@/lib/audit";
import { ProxyCallRow } from "@/components/ProxyCallRow";
import { ThinkingRow } from "@/components/audit/ThinkingRow";
import { EventRow } from "@/components/audit/EventRow";

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
    return (
      <ProxyCallRow call={row.call} mode={row.mode} name={name ?? ""} iteration={iteration ?? ""} />
    );
  }
  if (row.kind === "thinking") return <ThinkingRow row={row} open={open} onToggle={onToggle} />;
  return <EventRow row={row} open={open} onToggle={onToggle} />;
}
