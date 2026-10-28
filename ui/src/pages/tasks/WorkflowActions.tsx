import { Button } from "@/components/ui/button";
import type { WorkflowView } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { canReturnToAgent, canRunWatch, canUpgrade, hasActiveRun, isClosed } from "./workflowGraph";

export type OpenForm = "move" | "return" | "upgrade" | null;

/** Run watch now, Return to agent, Upgrade, Move, and Cancel, each where it applies. */
export default function WorkflowActions({
  view,
  upgradeTo,
  cancelling,
  watching,
  onRunWatch,
  onOpen,
  onCancel,
}: {
  view: WorkflowView;
  upgradeTo: string;
  cancelling: boolean;
  watching: boolean;
  onRunWatch: () => void;
  onOpen: (form: OpenForm) => void;
  onCancel: () => void;
}) {
  return (
    <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
      {canRunWatch(view) && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7"
          disabled={watching || hasActiveRun(view)}
          title={hasActiveRun(view) ? "A script of this task is running" : undefined}
          onClick={onRunWatch}
        >
          Run watch now
        </Button>
      )}
      {canReturnToAgent(view) && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7"
          onClick={() => onOpen("return")}
        >
          Return to agent…
        </Button>
      )}
      {canUpgrade(view) && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7"
          onClick={() => onOpen("upgrade")}
        >
          Upgrade to {upgradeTo}…
        </Button>
      )}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-7"
        onClick={() => onOpen("move")}
      >
        Move to status…
      </Button>
      {!isClosed(view) && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 text-destructive hover:text-destructive"
          disabled={cancelling}
          onClick={onCancel}
        >
          Cancel task
        </Button>
      )}
    </div>
  );
}

/** The first refusal of a header action, under the header. */
export function ActionError({ errors }: { errors: unknown[] }) {
  const error = errors.find(Boolean);
  if (!error) return null;
  return (
    <p role="alert" className="text-ui text-status-failed">
      {errorText(error)}
    </p>
  );
}
