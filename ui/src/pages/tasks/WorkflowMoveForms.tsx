import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { WorkflowView } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { cn } from "@/lib/utils";
import { FIELD, LABEL, SelectShell } from "./panelStyles";
import { useMounted } from "./workflowShared";

/**
 * Sends a form's confirmed write: `run` resolves true once done, false when
 * the confirmation is declined. The form closes once done, and shows why a
 * write failed.
 */
function useReasonSubmit(onDone: () => void) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const mountedRef = useMounted();
  const submit = (run: () => Promise<boolean>) => {
    setError("");
    setBusy(true);
    run()
      .then(
        (done) => {
          if (done && mountedRef.current) onDone();
        },
        (failure) => {
          if (mountedRef.current) setError(errorText(failure));
        },
      )
      .finally(() => {
        if (mountedRef.current) setBusy(false);
      });
  };
  return { error, busy, submit };
}

/**
 * The statuses a move may target: every declared status but the current one,
 * the terminal ones last. A move to a terminal status is the operator's
 * decision, so they stay on offer.
 */
function moveTargets(view: WorkflowView) {
  const others = (view.statuses ?? []).filter((status) => status.id !== view.status);
  return {
    open: others.filter((status) => !status.terminal),
    terminal: others.filter((status) => status.terminal),
  };
}

export function MoveForm({
  view,
  onMove,
  onDone,
}: {
  view: WorkflowView;
  onMove: (to: string, reason: string) => Promise<boolean>;
  onDone: () => void;
}) {
  const [to, setTo] = useState("");
  const [reason, setReason] = useState("");
  const { error, busy, submit } = useReasonSubmit(onDone);
  const { open, terminal } = moveTargets(view);
  return (
    <form
      className="flex min-w-0 flex-col gap-1.5 rounded-md bg-muted/50 p-2.5"
      onSubmit={(event) => {
        event.preventDefault();
        submit(() => onMove(to, reason.trim()));
      }}
    >
      <span className={LABEL}>Move to status</span>
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <SelectShell
          autoFocus
          aria-label="Target status"
          value={to}
          className="h-6.5 w-auto max-w-full text-ui md:text-ui"
          onChange={(event) => setTo(event.target.value)}
        >
          <option value="">Choose a status</option>
          {open.map((status) => (
            <option key={status.id} value={status.id}>
              {status.id}
            </option>
          ))}
          {terminal.length > 0 && (
            <optgroup label="Terminal">
              {terminal.map((status) => (
                <option key={status.id} value={status.id}>
                  {status.id}
                </option>
              ))}
            </optgroup>
          )}
        </SelectShell>
        <Input
          aria-label="Reason"
          placeholder="Reason (required)"
          value={reason}
          className={cn(FIELD, "h-6.5 min-w-40 flex-1")}
          onChange={(event) => setReason(event.target.value)}
        />
        <Button type="submit" size="sm" className="h-6.5" disabled={busy || !to || !reason.trim()}>
          Move
        </Button>
        <Button type="button" variant="ghost" size="sm" className="h-6.5" onClick={onDone}>
          Close
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-ui text-status-failed">
          {error}
        </p>
      )}
    </form>
  );
}

export function UpgradeForm({
  onUpgrade,
  onDone,
}: {
  onUpgrade: (reason: string) => Promise<boolean>;
  onDone: () => void;
}) {
  const [reason, setReason] = useState("");
  const { error, busy, submit } = useReasonSubmit(onDone);
  return (
    <form
      className="flex min-w-0 flex-col gap-1.5 rounded-md bg-muted/50 p-2.5"
      onSubmit={(event) => {
        event.preventDefault();
        submit(() => onUpgrade(reason.trim()));
      }}
    >
      <span className={LABEL}>Upgrade workflow</span>
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <Input
          autoFocus
          aria-label="Upgrade reason"
          placeholder="Reason (optional)"
          value={reason}
          className={cn(FIELD, "h-6.5 min-w-40 flex-1")}
          onChange={(event) => setReason(event.target.value)}
        />
        <Button type="submit" size="sm" className="h-6.5" disabled={busy}>
          Upgrade
        </Button>
        <Button type="button" variant="ghost" size="sm" className="h-6.5" onClick={onDone}>
          Close
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-ui text-status-failed">
          {error}
        </p>
      )}
    </form>
  );
}
