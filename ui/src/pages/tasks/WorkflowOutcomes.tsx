import { ArrowRight, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { OutcomeView, WorkflowView } from "@/lib/api";
import { cn } from "@/lib/utils";
import { DANGER_FILL, EMPTY, LABEL, MONO } from "./panelStyles";
import { useWorkflowAdvance } from "./useWorkflowAdvance";
import { visibleNotice, type Notice } from "./workflowShared";

const RESULT_LABEL: Record<string, string> = {
  rejected: "Rejected",
  failed: "Failed",
  cancelled: "Cancelled",
};

/** Who decides a status that is not the customer's. */
const OWNER_NOTE: Record<string, string> = {
  script: "The status's watch script decides.",
};
const POOL_NOTE = "The agent decides; the outcomes are listed for reference.";

/**
 * The outcomes of the current status: each with its target, the artifacts it
 * requires (set or still missing), and its checks with what they verify. The
 * Desktop acts as the customer, so a customer status gets one button per
 * outcome; a pool or script status is listed read-only, since its holder or
 * its watch script decides. A request that settled without applying is
 * explained by the problem block once the view carries it.
 */
export default function WorkflowOutcomes({
  taskKey,
  view,
  onChanged,
  onRefresh,
}: {
  taskKey: string;
  view: WorkflowView;
  /** The transition applied: refetch the view and the task. */
  onChanged: () => void;
  /** Refetch the view only. */
  onRefresh: () => void;
}) {
  const customer = view.owner === "customer";
  const paused = view.waiting_on === "pause";
  const { message, setMessage, checking, notice, advance } = useWorkflowAdvance(
    taskKey,
    view,
    onChanged,
    onRefresh,
  );

  const last = view.last_request;
  const shown = visibleNotice(notice, last);
  const pendingElsewhere = !checking && last?.state === "pending";
  const disabled = checking || paused || pendingElsewhere;
  const some = view.outcomes.length > 0;

  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <span className={LABEL}>Outcomes</span>
      {!some && <span className={EMPTY}>This status has no outcomes.</span>}
      {!customer && some && <span className={EMPTY}>{OWNER_NOTE[view.owner] ?? POOL_NOTE}</span>}
      <ul className="flex min-w-0 flex-col gap-1.5">
        {view.outcomes.map((outcome) => (
          <OutcomeRow
            key={outcome.on}
            outcome={outcome}
            onAdvance={customer ? () => advance(outcome.on) : undefined}
            disabled={disabled}
          />
        ))}
      </ul>
      {customer && some && (
        <Textarea
          aria-label="Outcome message"
          placeholder="Message (optional)"
          value={message}
          disabled={checking}
          maxLength={4096}
          onChange={(event) => setMessage(event.target.value)}
          className="min-h-12 rounded-md border-0 bg-muted text-body-sm md:text-body-sm"
        />
      )}
      {(checking || pendingElsewhere) && (
        <CheckingStatus onRefresh={pendingElsewhere ? onRefresh : undefined} />
      )}
      {shown && <OutcomeNotice notice={shown} onRefresh={onRefresh} />}
    </section>
  );
}

/**
 * One outcome: a button where the customer declares it, its name otherwise,
 * then its target, the artifacts it requires, and its checks.
 */
function OutcomeRow({
  outcome,
  onAdvance,
  disabled,
}: {
  outcome: OutcomeView;
  /** Declares the outcome; absent where the customer does not decide. */
  onAdvance?: () => void;
  disabled: boolean;
}) {
  const missing = new Set(outcome.missing ?? []);
  return (
    <li className="flex min-w-0 flex-col gap-0.5">
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        {onAdvance ? (
          <Button
            type="button"
            size="sm"
            className="h-7"
            disabled={disabled || missing.size > 0}
            onClick={onAdvance}
          >
            {outcome.on}
          </Button>
        ) : (
          <span className={cn(MONO, "font-medium")}>{outcome.on}</span>
        )}
        {onAdvance && missing.size > 0 && (
          <span className="text-meta text-status-failed">needs: {[...missing].join(", ")}</span>
        )}
        <ArrowRight aria-hidden="true" className="size-3 text-muted-foreground" />
        <span className={cn(MONO, "min-w-0 break-all")}>{outcome.to}</span>
        <RequiredArtifacts names={outcome.requires ?? []} missing={missing} />
      </div>
      <Checks outcome={outcome} />
    </li>
  );
}

/** The artifacts an outcome requires, each marked set or still missing. */
function RequiredArtifacts({ names, missing }: { names: string[]; missing: Set<string> }) {
  if (names.length === 0) return null;
  return (
    <>
      <span className="text-meta text-muted-foreground">requires</span>
      {names.map((name) =>
        missing.has(name) ? (
          <span
            key={name}
            data-missing="true"
            title="No value yet"
            className={cn(MONO, "rounded-chip px-1.5", DANGER_FILL)}
          >
            {name}
          </span>
        ) : (
          <span
            key={name}
            data-present="true"
            title="Set"
            className={cn(MONO, "inline-flex items-center gap-0.5")}
          >
            <Check aria-hidden="true" className="size-2.5 text-status-running [stroke-width:2.5]" />
            {name}
          </span>
        ),
      )}
    </>
  );
}

/** A request is running; Refresh is offered when it was sent from elsewhere. */
export function CheckingStatus({ onRefresh }: { onRefresh?: () => void }) {
  return (
    <div className="flex items-center gap-2">
      <span role="status" className={EMPTY}>
        checking…
      </span>
      {onRefresh && (
        <Button type="button" variant="ghost" className="h-6 px-2 text-ui" onClick={onRefresh}>
          Refresh
        </Button>
      )}
    </div>
  );
}

/** The checks of an outcome: each script with what it verifies, when the image says. */
function Checks({ outcome }: { outcome: OutcomeView }) {
  const checks = outcome.checks ?? [];
  if (checks.length === 0) return null;
  return (
    <ul className="flex min-w-0 flex-col gap-0.5 pl-3">
      {checks.map((script, index) => {
        const detail = outcome.check_details?.[index];
        return (
          <li key={script} className="flex min-w-0 flex-wrap items-baseline gap-x-2 text-meta">
            <span className="text-muted-foreground">check</span>
            <span className={cn(MONO, "min-w-0 break-all text-muted-foreground")}>{script}</span>
            {detail?.description && (
              <span className="min-w-0 break-words">{detail.description}</span>
            )}
            {detail && (
              <span className={cn(MONO, "text-muted-foreground opacity-80")}>
                as {detail.run_as}, timeout {detail.timeout}
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}

export function OutcomeNotice({ notice, onRefresh }: { notice: Notice; onRefresh: () => void }) {
  if (notice.kind === "timeout") {
    return (
      <div role="status" className="flex items-center gap-2 text-ui text-muted-foreground">
        Still checking — the result appears here when the checks finish.
        <Button type="button" variant="ghost" className="h-6 px-2 text-ui" onClick={onRefresh}>
          Refresh
        </Button>
      </div>
    );
  }
  const text = notice.kind === "error" ? notice.message : notice.request.result_message;
  const label =
    notice.kind === "error"
      ? notice.label
      : (RESULT_LABEL[notice.request.state] ?? notice.request.state);
  return (
    <div
      role="alert"
      className={cn("flex min-w-0 flex-col gap-1 rounded-md px-2.5 py-2 text-ui", DANGER_FILL)}
    >
      <span className="font-medium">
        {label}
        {notice.kind === "result" ? ` · ${notice.request.outcome}` : ""}
      </span>
      {text && (
        <pre className="min-w-0 font-mono text-meta whitespace-pre-wrap break-words">{text}</pre>
      )}
    </div>
  );
}
