import type { RefObject } from "react";
import { useNavigate } from "react-router-dom";
import { Container } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { StatusDot, StatusPill } from "@/components/ui/status";
import { AgentControls } from "@/components/AgentControls";
import { GoalHelp } from "@/components/GoalHelp";
import type { DaemonCtx } from "@/components/daemonContext";
import { agentTone } from "@/lib/statusTone";
import { openHostPathInVSCode } from "@/lib/desktop";
import { cn } from "@/lib/utils";
import type { AgentBudget, AgentRow, AgentStatus, AgentView } from "@/lib/api";
import { canOpenAgentCwdInVSCode } from "./agentCwdVSCode";
import { agentRuntimeLabel, hasBudgetLimits, type AgentTabEntry } from "./agentWorkspaceModel";
import { AgentWorkspaceTabs } from "./AgentWorkspaceTabs";

type AgentWorkspaceHeaderProps = {
  hostId: string;
  hostLabel?: string;
  agent: AgentRow;
  status: AgentStatus | null;
  view: AgentView | null;
  daemons: DaemonCtx["daemons"];
  unavailable: boolean;
  base: string;
  tabs: readonly AgentTabEntry[];
  stripRef: RefObject<HTMLElement | null>;
  questionTasks: number;
  refresh: () => void;
  onExec: () => void;
  onOpenGoal: (taskKey: string) => void;
};

function AgentGoal({
  goalTaskKey,
  onOpenGoal,
}: {
  goalTaskKey?: string;
  onOpenGoal: (taskKey: string) => void;
}) {
  return (
    <span className="flex min-w-0 flex-1 items-center gap-1.25 overflow-hidden text-body-sm whitespace-nowrap text-muted-foreground max-[75rem]:flex-none">
      <span className="shrink-0">Goal:</span>
      {goalTaskKey ? (
        <button
          type="button"
          className="shrink-0 border-b border-dotted border-border font-mono text-meta font-medium text-foreground tabular-nums hover:border-foreground"
          onClick={() => onOpenGoal(goalTaskKey)}
        >
          {goalTaskKey}
        </button>
      ) : (
        <span className="shrink-0">No current goal</span>
      )}
      <GoalHelp />
    </span>
  );
}

function AgentIdentityRow({
  agent,
  hostId,
  hostLabel,
  outOfBudget,
  runtime,
  onOpenGoal,
}: {
  agent: AgentRow;
  hostId: string;
  hostLabel?: string;
  outOfBudget: boolean;
  runtime: string;
  onOpenGoal: (taskKey: string) => void;
}) {
  const tone = agentTone(agent.state, outOfBudget);
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-2.25 gap-y-1 min-[75rem]:flex-nowrap">
      <StatusDot tone={tone} size={8} />
      <h1 className="min-w-0 truncate text-title font-semibold tracking-[-.01em] min-[75rem]:shrink-0">
        {agent.name}
      </h1>
      <StatusPill tone={tone}>{outOfBudget ? "no budget" : agent.state}</StatusPill>
      <span className="shrink-0 text-body-sm whitespace-nowrap text-muted-foreground">
        {hostLabel || (hostId ? hostId : "Local")}
      </span>
      <span
        title={agent.image}
        data-testid="agent-image"
        className="inline-flex h-5 max-w-full min-w-0 items-center gap-1.25 rounded-sm bg-muted px-1.75 font-mono text-meta whitespace-nowrap text-muted-foreground min-[75rem]:shrink-0"
      >
        <Container className="size-2.5 shrink-0" aria-hidden="true" />
        <span className="truncate">{agent.image}</span>
      </span>
      {runtime && (
        <span
          data-testid="agent-runtime"
          title={`Harness · model · effort: ${runtime}`}
          className="inline-flex h-5 max-w-full min-w-0 items-center rounded-sm bg-muted px-1.75 font-mono text-meta whitespace-nowrap text-muted-foreground min-[75rem]:shrink-0"
        >
          <span className="truncate">{runtime}</span>
        </span>
      )}
      <AgentGoal goalTaskKey={agent.current_goal_task_key} onOpenGoal={onOpenGoal} />
    </div>
  );
}

// The reference ellipsises the path to hold the header to one line; this app
// shows it whole on purpose (a wrapped cwd is better than a path the operator
// cannot read), so it wraps.
function AgentCwdRow({
  hostId,
  cwd,
  daemons,
  unavailable,
}: {
  hostId: string;
  cwd: string;
  daemons: DaemonCtx["daemons"];
  unavailable: boolean;
}) {
  return (
    <div className="flex min-w-0 flex-wrap items-baseline gap-x-2.5 gap-y-1 text-ui">
      <span className="flex min-w-0 items-baseline gap-1.5">
        <span className="shrink-0 text-muted-foreground">cwd:</span>
        <span data-testid="agent-cwd" className="min-w-0 break-all font-mono text-meta">
          {cwd || "…"}
        </span>
      </span>
      {cwd && canOpenAgentCwdInVSCode(hostId, daemons) && (
        <Button
          type="button"
          data-testid="open-agent-cwd-vscode"
          variant="link"
          size="xs"
          className="h-auto shrink-0 px-0 py-0 text-ui font-medium text-foreground"
          disabled={unavailable}
          onClick={() =>
            void openHostPathInVSCode(hostId, cwd).catch((error) => toast.error(String(error)))
          }
        >
          Open in VS Code
        </Button>
      )}
    </div>
  );
}

// Why the loop is off: a halted loop (including a failed iteration launch,
// which never creates an iteration row), and a full inbox.
function AgentStatusNotes({ status }: { status: AgentStatus | null }) {
  return (
    <>
      {(status?.halt_reason || status?.error_reason) && (
        <p
          data-testid="agent-halt-reason"
          className={cn(
            "px-4 pb-2 text-caption font-medium",
            status.halt_kind === "idle_limit" ? "text-muted-foreground" : "text-destructive",
          )}
        >
          {status.halt_reason || status.error_reason}
        </p>
      )}
      {status?.messages_queue_full && (
        <p className="px-4 pb-2 text-caption font-medium text-destructive">
          Inbox full: {status.messages_pending} / {status.messages_max_queue}
        </p>
      )}
    </>
  );
}

function AgentBudgetLine({ budget, exhausted }: { budget: AgentBudget; exhausted: string[] }) {
  return (
    <div className="px-4 pb-2 text-caption" data-testid="agent-budget-header">
      {exhausted.length > 0 && (
        <p className="font-medium text-destructive">Out of budget: {exhausted.join(", ")}</p>
      )}
      <p className="text-muted-foreground tabular-nums">
        Hour {budget.hour_spent_usd.toFixed(2)} / {budget.hour_usd || "Unlimited"} · Day{" "}
        {budget.day_spent_usd.toFixed(2)} / {budget.day_usd || "Unlimited"} · Week{" "}
        {budget.week_spent_usd.toFixed(2)} / {budget.week_usd || "Unlimited"} · Month{" "}
        {budget.month_spent_usd.toFixed(2)} / {budget.month_usd || "Unlimited"}
      </p>
    </div>
  );
}

// Agent header: one line of identity (who, how it is doing, where it runs, what
// it is working toward), one line of location, and the lifecycle cluster —
// visible on every tab.
export function AgentWorkspaceHeader(props: AgentWorkspaceHeaderProps) {
  const { hostId, agent, status, view, unavailable, base } = props;
  const navigate = useNavigate();
  const exhaustedPeriods = status?.budget?.exhausted ?? agent.budget?.exhausted ?? [];
  const cwd = agent.cwd || view?.cwd || "";
  const runtime = agentRuntimeLabel({ agentHarness: agent.harness, view });
  return (
    <header className="shrink-0">
      {/* Below 1200px the identity wraps and the lifecycle cluster takes a row of
          its own, so neither ever covers the other. */}
      <div className="flex flex-wrap items-start gap-2.5 px-4 pt-3.5 pb-2.75 min-[75rem]:flex-nowrap">
        <div className="flex min-w-0 flex-1 flex-col gap-1.25 max-[75rem]:basis-full">
          <AgentIdentityRow
            agent={agent}
            hostId={hostId}
            hostLabel={props.hostLabel}
            outOfBudget={exhaustedPeriods.length > 0}
            runtime={runtime}
            onOpenGoal={props.onOpenGoal}
          />
          <AgentCwdRow
            hostId={hostId}
            cwd={cwd}
            daemons={props.daemons}
            unavailable={unavailable}
          />
        </div>
        <AgentControls
          name={agent.name}
          alive={agent.enabled ?? agent.state !== "stopped"}
          image={agent.image}
          disabled={unavailable}
          configurationPath={`${base}/configuration`}
          refresh={props.refresh}
          onExec={props.onExec}
          onDeleted={() => navigate("/")}
        />
      </div>
      <AgentStatusNotes status={status} />
      {hasBudgetLimits(status?.budget) && (
        <AgentBudgetLine budget={status.budget} exhausted={exhaustedPeriods} />
      )}
      <AgentWorkspaceTabs
        tabs={props.tabs}
        base={base}
        questionTasks={props.questionTasks}
        openLetters={status?.messages_pending ?? 0}
        stripRef={props.stripRef}
      />
    </header>
  );
}
