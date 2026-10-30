import { useEffect, useState, type RefObject } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { StatusDot } from "@/components/ui/status";
import { AgentControls } from "@/components/AgentControls";
import type { DaemonCtx } from "@/components/daemonContext";
import { agentStateTone, type AgentState } from "@/lib/agentState";
import { openHostPathInVSCode } from "@/lib/desktop";
import { cn } from "@/lib/utils";
import type { AgentRow, AgentStatus, AgentView } from "@/lib/api";
import { togglePinnedAgent, usePinnedAgents } from "@/pages/terminals/sidebar/sidebarPrefs";
import { agentKey } from "@/pages/terminals/sidebar/sidebarModel";
import { canOpenAgentCwdInVSCode } from "./agentCwdVSCode";
import {
  agentRuntimeLabel,
  hasBudgetLimits,
  headerSentence,
  primaryAction,
} from "./agentWorkspaceModel";
import { AgentWorkspaceTabs } from "./AgentWorkspaceTabs";

const MINUTE_MS = 60_000;

type AgentWorkspaceHeaderProps = {
  hostId: string;
  agent: AgentRow;
  status: AgentStatus | null;
  view: AgentView | null;
  agentState: AgentState;
  /** What the agent waits on the user for, the Work tab's badge. */
  needsCount: number;
  daemons: DaemonCtx["daemons"];
  unavailable: boolean;
  base: string;
  stripRef: RefObject<HTMLElement | null>;
  refresh: () => void;
  onExec: () => void;
  onClone?: () => void;
};

// The muted third line: harness · model · effort, the folder (cut from the
// left, so its last names stay readable), and today's spend when it matters.
function AgentFacts({
  runtime,
  cwd,
  status,
}: {
  runtime: string;
  cwd: string;
  status: AgentStatus | null;
}) {
  const budget = status?.budget;
  const spent = budget?.day_spent_usd ?? 0;
  const spend = hasBudgetLimits(budget) || spent > 0 ? `$${spent.toFixed(2)} today` : "";
  return (
    <p
      data-testid="agent-facts"
      className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 text-caption text-muted-foreground"
    >
      {runtime && <span data-testid="agent-runtime">{runtime}</span>}
      {cwd && (
        <>
          {runtime && <span aria-hidden="true">·</span>}
          <span
            data-testid="agent-cwd"
            title={cwd}
            dir="rtl"
            className="max-w-full min-w-0 truncate text-left font-mono text-meta"
          >
            {/* dir=rtl cuts the start; bdi keeps the path itself left to right. */}
            <bdi>{cwd}</bdi>
          </span>
        </>
      )}
      {spend && (
        <>
          <span aria-hidden="true">·</span>
          <span className="tabular-nums">{spend}</span>
        </>
      )}
    </p>
  );
}

// Agent header: the name with the one primary action, one sentence on what
// the agent does or waits for, and a muted line of facts — on every tab.
export function AgentWorkspaceHeader(props: AgentWorkspaceHeaderProps) {
  const { hostId, agent, status, view, agentState, unavailable, base } = props;
  const navigate = useNavigate();
  const pinned = usePinnedAgents();
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), MINUTE_MS);
    return () => window.clearInterval(timer);
  }, []);
  const cwd = agent.cwd || view?.cwd || "";
  const runtime = agentRuntimeLabel({ agentHarness: agent.harness, view });
  const primary = primaryAction(agentState);
  const answerKey = agentState.kind === "needs" ? agentState.needs.first?.key : undefined;
  const pinKey = agentKey(hostId, agent.name);
  const sentence = headerSentence(agentState, {
    goalTaskKey: agent.current_goal_task_key,
    runs: status?.iterations ?? 0,
    now,
  });
  return (
    <header className="shrink-0">
      {/* Below 1200px the name and the sentence take the full width and the
          buttons a row of their own, so neither ever covers the other. */}
      <div className="flex flex-wrap items-start gap-2.5 px-4 pt-3.5 pb-2.75 min-[75rem]:flex-nowrap">
        <div className="flex min-w-0 flex-1 flex-col gap-1 max-[75rem]:basis-full">
          <div className="flex min-w-0 items-center gap-2.25">
            <StatusDot tone={agentStateTone(agentState)} size={8} />
            <h1 className="min-w-0 truncate text-title font-semibold tracking-[-.01em]">
              {agent.name}
            </h1>
          </div>
          <p
            data-testid="agent-sentence"
            className={cn(
              "text-body-sm",
              agentState.kind === "error" ? "font-medium text-destructive" : "text-foreground",
            )}
          >
            {sentence}
          </p>
          {status?.halt_kind === "idle_limit" && status.halt_reason && (
            // The loop stopped as configured: said, not alarmed.
            <p data-testid="agent-halt-reason" className="text-caption text-muted-foreground">
              {status.halt_reason}
            </p>
          )}
          <AgentFacts runtime={runtime} cwd={cwd} status={status} />
          {status?.messages_queue_full && (
            <p className="text-caption font-medium text-destructive">
              Inbox full: {status.messages_pending} / {status.messages_max_queue}
            </p>
          )}
        </div>
        <AgentControls
          name={agent.name}
          alive={status?.enabled ?? agent.enabled ?? agent.state !== "stopped"}
          image={agent.image}
          disabled={unavailable}
          primary={primary}
          workPath={
            answerKey ? `${base}/work?task=${encodeURIComponent(answerKey)}` : `${base}/work`
          }
          settingsPath={`${base}/settings`}
          refresh={props.refresh}
          onExec={props.onExec}
          onDeleted={() => navigate("/")}
          onClone={props.onClone}
          pinned={pinned.has(pinKey)}
          onTogglePin={() => togglePinnedAgent(pinKey)}
          onOpenFolder={
            cwd && canOpenAgentCwdInVSCode(hostId, props.daemons)
              ? () =>
                  void openHostPathInVSCode(hostId, cwd).catch((error) =>
                    toast.error(String(error)),
                  )
              : undefined
          }
        />
      </div>
      <AgentWorkspaceTabs base={base} needsCount={props.needsCount} stripRef={props.stripRef} />
    </header>
  );
}
