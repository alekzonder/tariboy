import { transferSummaryText, type TransferPhase, type TransferRow } from "./transferStatus";

export function TransferSummary(props: {
  phase: TransferPhase;
  rows: Record<string, TransferRow>;
  targetCount: number;
  selectedCount: number;
}) {
  return (
    <p aria-live="polite" className="text-sm text-muted-foreground">
      {transferSummaryText(props)}
    </p>
  );
}
