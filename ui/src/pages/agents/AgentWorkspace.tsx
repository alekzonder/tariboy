import { useCallback, useEffect, useState } from "react";
import { NavLink, Navigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { useDaemons } from "@/components/DaemonProvider";
import { Button } from "@/components/ui/button";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import { agentGetOn } from "@/lib/api";
import { openHostPathInVSCode } from "@/lib/desktop";
import { cn } from "@/lib/utils";
import { hostToParam, targetFor } from "@/lib/terminalsHost";
import type { AgentStatus, AgentSummary, AgentView } from "@/lib/types";
import AgentConsoleTab from "./AgentConsoleTab";
import AgentAutopilotTab from "./AgentAutopilotTab";
import AgentActivityTab from "./AgentActivityTab";
import AgentConfigurationTab from "./AgentConfigurationTab";
import AgentAdvancedTab from "./AgentAdvancedTab";
import TasksWorkspace from "@/pages/tasks/TasksWorkspace";
import { useCustomerQuestionNotifications } from "@/components/customerQuestionNotificationsContext";
import { canOpenAgentCwdInVSCode } from "./agentCwdVSCode";

const TABS = [
  ["console", "Console"],
  ["autopilot", "Autopilot"],
  ["activity", "Activity"],
  ["tasks", "Tasks"],
  ["configuration", "Configuration"],
  ["advanced", "Advanced"],
] as const;

export default function AgentWorkspace({ hostId, hostLabel, agent, refresh, unavailable = false }: {
  hostId: string;
  hostLabel?: string;
  agent: AgentSummary;
  refresh: () => void;
  unavailable?: boolean;
}) {
  const { tab = "console" } = useParams();
  const { activeId, daemons, select } = useDaemons();
  const [connection, setConnection] = useState<"selecting" | "ready" | "unavailable">("selecting");
  const [status, setStatus] = useState<AgentStatus | null>(null);
  const [resolvedCwd, setResolvedCwd] = useState({ key: "", cwd: "" });
  const cwdKey = `${hostId}\0${agent.name}\0${agent.cwd ?? ""}`;
  const effectiveCwd = agent.cwd || (resolvedCwd.key === cwdKey ? resolvedCwd.cwd : "");
  const target = targetFor(hostId);
  const { refreshHost } = useCustomerQuestionNotifications();

  useEffect(() => {
    let cancelled = false;
    void select(hostId).then((ok) => {
      if (!cancelled) setConnection(ok ? "ready" : "unavailable");
    });
    return () => { cancelled = true; };
  }, [hostId, select]);

  // The URL is authoritative inside an agent workspace. A global host change
  // must never retarget legacy tab components to a same-named agent elsewhere.
  // Hide the tabs immediately and restore the host named by the route.
  useEffect(() => {
    if (connection !== "ready" || activeId === hostId) return;
    let cancelled = false;
    void select(hostId).then((ok) => {
      if (!cancelled && !ok) setConnection("unavailable");
    });
    return () => { cancelled = true; };
  }, [activeId, connection, hostId, select]);

  const refreshStatus = useCallback(async () => {
    if (connection !== "ready" || unavailable) return;
    const requestTarget = targetFor(hostId);
    try {
      setStatus(await agentGetOn<AgentStatus>(requestTarget, agent.name, "status"));
    } catch {
      setStatus(null);
    }
    if (!agent.cwd) {
      try {
        const view = await agentGetOn<AgentView>(requestTarget, agent.name, "");
        setResolvedCwd({ key: cwdKey, cwd: view.cwd });
      } catch {
        // Preserve the last successful value and retry with the next status poll.
      }
    }
  }, [agent.cwd, agent.name, connection, cwdKey, hostId, unavailable]);

  useEffect(() => {
    if (connection !== "ready" || unavailable) return;
    void Promise.resolve().then(refreshStatus);
    const timer = window.setInterval(() => void refreshStatus(), 3000);
    return () => window.clearInterval(timer);
  }, [connection, refreshStatus, unavailable]);

  if (!TABS.some(([key]) => key === tab)) {
    return <Navigate to={`/agents/${hostToParam(hostId)}/${encodeURIComponent(agent.name)}/console`} replace />;
  }
  if (connection === "unavailable") {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
        <p>Host unavailable.</p>
        <p>The route was not reassigned to the local daemon.</p>
      </div>
    );
  }
  if (connection === "selecting") {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Connecting to host…</div>;
  }
  if (activeId !== hostId) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Restoring route host…</div>;
  }

  const base = `/agents/${hostToParam(hostId)}/${encodeURIComponent(agent.name)}`;
  const content =
    tab === "console" ? <AgentConsoleTab hostId={hostId} agent={agent} refresh={refresh} />
    : tab === "autopilot" ? <AgentAutopilotTab />
    : tab === "activity" ? <AgentActivityTab />
    : tab === "tasks" ? <TasksWorkspace
      scopeAgent={agent.name}
      target={target}
      onNotificationsChanged={() => void refreshHost(hostId)}
    />
    : tab === "configuration" ? <AgentConfigurationTab target={target} refresh={refresh} />
    : <AgentAdvancedTab />;

  return (
    <AgentNameContext.Provider value={agent.name}>
      <AgentStatusContext.Provider value={{ status, refresh: refreshStatus }}>
        <div className="flex h-full min-h-0 flex-col">
          <header className="mb-3 shrink-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-lg font-semibold">{agent.name}</h1>
              <span className="text-sm text-muted-foreground">{agent.state}</span>
              <span className="text-sm text-muted-foreground">{hostLabel || (hostId ? hostId : "Local")}</span>
              <span className="min-w-0 truncate text-sm text-muted-foreground">{agent.image}</span>
            </div>
            <div className="mt-1 flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1 text-xs">
              <span className="font-mono text-muted-foreground">cwd:</span>
              <span data-testid="agent-cwd" className="min-w-0 break-all font-mono">
                {effectiveCwd || "…"}
              </span>
              {effectiveCwd && canOpenAgentCwdInVSCode(hostId, daemons) && (
                <Button
                  type="button"
                  data-testid="open-agent-cwd-vscode"
                  variant="link"
                  size="xs"
                  className="h-auto px-0 py-0 text-xs"
                  disabled={unavailable}
				  onClick={() => void openHostPathInVSCode(hostId, effectiveCwd)
                    .catch((error) => toast.error(String(error)))}
                >
                  Open in VS Code
                </Button>
              )}
            </div>
			{status?.budget && (status.budget.hour_usd > 0 || status.budget.day_usd > 0 || status.budget.week_usd > 0 || status.budget.month_usd > 0) && <div className="mt-2 text-xs" data-testid="agent-budget-header">
				{status.budget.exhausted.length > 0 && <p className="font-semibold text-destructive">Out of budget: {status.budget.exhausted.join(", ")}</p>}
				<p>Hour {status.budget.hour_spent_usd.toFixed(2)} / {status.budget.hour_usd || "Unlimited"} · Day {status.budget.day_spent_usd.toFixed(2)} / {status.budget.day_usd || "Unlimited"} · Week {status.budget.week_spent_usd.toFixed(2)} / {status.budget.week_usd || "Unlimited"} · Month {status.budget.month_spent_usd.toFixed(2)} / {status.budget.month_usd || "Unlimited"}</p>
			</div>}
            <nav aria-label="Agent workspace" className="mt-2 flex gap-1 border-b">
              {TABS.map(([key, label]) => (
                <NavLink
                  key={key}
                  to={`${base}/${key}`}
                  className={({ isActive }) =>
                    cn("border-b-2 border-transparent px-3 py-2 text-sm hover:text-foreground",
                      isActive && "border-primary font-medium")
                  }
                >
                  {label}
                </NavLink>
              ))}
            </nav>
          </header>
          {unavailable && (
            <p role="status" className="mb-2 text-sm text-muted-foreground">
              This host is temporarily unavailable; actions are disabled until it reconnects.
            </p>
          )}
          <section
            data-testid="agent-workspace-content"
            inert={unavailable}
            aria-disabled={unavailable || undefined}
            className="min-h-0 flex-1 overflow-auto"
          >
            {content}
          </section>
        </div>
      </AgentStatusContext.Provider>
    </AgentNameContext.Provider>
  );
}
