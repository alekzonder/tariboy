import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { transferStatusLabel, type TransferRow } from "./transferStatus";
import type { DestinationTarget } from "./useImageTransfer";

interface ProgressActions {
  disabled: boolean;
  onRetryRefChange: (id: string, retryRef: string) => void;
  onRetry: (id: string) => void;
}

function TransferProgressRow({
  target,
  row,
  disabled,
  onRetryRefChange,
  onRetry,
}: ProgressActions & { target: DestinationTarget; row: TransferRow }) {
  return (
    <li className="text-sm">
      <p>
        {target.label}: {transferStatusLabel(row.status)}
        {row.error ? ` — ${row.error}` : ""}
      </p>
      {row.retryRef !== undefined && (
        <div className="mt-1 flex gap-2">
          <Input
            aria-label={`Retag and retry for ${target.label}`}
            value={row.retryRef}
            disabled={disabled}
            onChange={(event) => onRetryRefChange(target.id, event.target.value)}
          />
          <Button
            size="sm"
            onClick={() => onRetry(target.id)}
            disabled={disabled || !row.retryRef.trim()}
          >
            Retag and retry {target.label}
          </Button>
        </div>
      )}
    </li>
  );
}

export function TransferProgressList({
  targets,
  rows,
  ...actions
}: ProgressActions & { targets: DestinationTarget[]; rows: Record<string, TransferRow> }) {
  return (
    <ul className="space-y-2" aria-label="Transfer progress">
      {targets
        .filter((target) => rows[target.id])
        .map((target) => (
          <TransferProgressRow key={target.id} target={target} row={rows[target.id]} {...actions} />
        ))}
    </ul>
  );
}
