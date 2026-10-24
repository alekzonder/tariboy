type TransferStatus =
  | "queued"
  | "exporting"
  | "previewing"
  | "importing"
  | "completed"
  | "already-present"
  | "failed"
  | "cancelled";

export interface TransferRow {
  status: TransferStatus;
  importID?: string;
  error?: string;
  retryRef?: string;
}

export type TransferPhase = "idle" | "exporting" | "transferring";

export function transferStatusLabel(status: TransferStatus): string {
  return status === "already-present"
    ? "Already present"
    : status[0].toUpperCase() + status.slice(1);
}

const completedRows = (rows: Record<string, TransferRow>) =>
  Object.values(rows).filter(
    (row) => row.status === "completed" || row.status === "already-present",
  ).length;

export function transferSummaryText({
  phase,
  rows,
  targetCount,
  selectedCount,
}: {
  phase: TransferPhase;
  rows: Record<string, TransferRow>;
  targetCount: number;
  selectedCount: number;
}): string {
  if (phase === "exporting") return "Exporting source archive.";
  if (phase === "transferring") {
    const completed = completedRows(rows);
    return `Transferring image: ${completed} target${completed === 1 ? "" : "s"} completed.`;
  }
  if (targetCount === 0) return "No transfer targets selected.";
  return `${selectedCount} transfer target${selectedCount === 1 ? "" : "s"} selected.`;
}
