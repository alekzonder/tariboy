import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import AuditLogPage from "@/pages/AuditLogPage";
import AgentUsagePage from "@/pages/AgentUsagePage";
import { useApi } from "@/components/apiContext";
import { useAgentName } from "@/lib/agent";
import { useActivityWindow } from "@/lib/activityPeriod";
import type { StatusHistoryEvent } from "@/lib/types";
import { fmtDateTime } from "@/lib/time";
import { useTaskActivity } from "@/hooks/useTaskActivity";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import {
  ActivityPeriodBar,
  ActivityTabs,
  ActivityTotals,
  TaskActivityTable,
  TruncatedNote,
} from "@/pages/activity/ActivityParts";
import TabPage from "@/components/TabPage";

type View = "tasks" | "iterations" | "usage" | "status";
const VIEWS: Array<{ view: View; label: string }> = [
  { view: "tasks", label: "Tasks" },
  { view: "iterations", label: "Iterations" },
  { view: "usage", label: "Usage" },
  { view: "status", label: "Status history" },
];

function viewFrom(params: URLSearchParams): View {
  const raw = params.get("view");
  if (raw === "tasks" || raw === "iterations" || raw === "usage" || raw === "status") return raw;
  // An ?iteration= deep link (e.g. from Usage) opens the iteration list.
  return params.has("iteration") ? "iterations" : "tasks";
}

export default function AgentActivityTab() {
  const name = useAgentName();
  const { window, setPeriod, setCustom } = useActivityWindow();
  const [searchParams, setSearchParams] = useSearchParams();
  const view = viewFrom(searchParams);
  const api = useApi();
  const { report, error } = useTaskActivity(window, name);
  const [taskKey, setTaskKey] = useState("");
  // An invalid custom range shows its error and leaves the views unbounded.
  const bounds = { since: window.since, until: window.until };

  const [history, setHistory] = useState<StatusHistoryEvent[]>([]);
  useEffect(() => {
    if (view !== "status") return;
    void api.agents.getStatusHistory(name).then((result) => setHistory(result.events)).catch(() => setHistory([]));
  }, [api, name, view]);
  const inPeriod = history.filter((event) => {
    const at = Date.parse(event.ts);
    return !bounds.since || (at >= Date.parse(bounds.since) && at < Date.parse(bounds.until));
  });

  const selectView = (next: View) => {
    const params = new URLSearchParams(searchParams);
    params.set("view", next);
    setSearchParams(params, { replace: true });
  };

  return (
    <TabPage className="overflow-auto">
      <ActivityPeriodBar window={window} onPeriod={setPeriod} onCustom={setCustom} />
      <ActivityTotals totals={report?.totals ?? null} />
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <ActivityTabs views={VIEWS} active={view} onSelect={selectView} />
      <div className="min-h-0 flex-1">
        {view === "tasks" ? (
          <div className="flex flex-col gap-2">
            <TaskActivityTable
              rows={report?.tasks ?? []}
              onOpenTask={setTaskKey}
            />
            {report?.truncated && <TruncatedNote shown={report.tasks.length} />}
          </div>
        ) : view === "iterations" ? (
          <AuditLogPage period={bounds} />
        ) : view === "usage" ? (
          <AgentUsagePage period={bounds} />
        ) : (
          <div className="mx-auto max-w-3xl space-y-2">
            {inPeriod.map((event, index) => (
              <div key={`${event.ts}-${index}`} className="rounded-md border p-3 text-sm">
                <div className="text-xs text-muted-foreground">{fmtDateTime(event.ts)}</div>
                <div>{event.message}</div>
              </div>
            ))}
            {inPeriod.length === 0 && <p className="text-sm text-muted-foreground">No status changes in this period.</p>}
          </div>
        )}
      </div>
      {taskKey && <TaskDrawer key={taskKey} taskKey={taskKey} onClose={() => setTaskKey("")} />}
    </TabPage>
  );
}
