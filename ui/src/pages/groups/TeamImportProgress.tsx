import type { TeamImportOperation } from "@/lib/api";
import { Button } from "@/components/ui/button";

interface Props {
  operation: TeamImportOperation;
  canRetry: boolean;
  onRetry: () => void;
}

export function TeamImportProgress({ operation, canRetry, onRetry }: Props) {
  return (
    <div className="space-y-1 text-xs" aria-label="Team import progress">
      <div>Import status: {operation.status}</div>
      {operation.steps.map((step, index) => (
        <div key={`${step.kind}-${step.name}-${index}`}>
          {step.kind} {step.name}: {step.status}
          {step.error ? ` — ${step.error}` : ""}
        </div>
      ))}
      {operation.status === "failed" && canRetry && (
        <Button size="sm" variant="outline" onClick={onRetry}>
          Retry unfinished work
        </Button>
      )}
    </div>
  );
}
