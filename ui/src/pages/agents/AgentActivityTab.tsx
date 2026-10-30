import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import AuditLogPage from "@/pages/AuditLogPage";
import AgentUsagePage from "@/pages/AgentUsagePage";
import { useApi } from "@/components/apiContext";
import { Segmented } from "@/components/ui/segmented";
import { usePolling } from "@/hooks/usePolling";
import { useAgentName } from "@/lib/agent";
import { AGENT_ACTIVITY_PERIODS, useActivityWindow } from "@/lib/activityPeriod";
import { useTaskActivity } from "@/hooks/useTaskActivity";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import {
  ActivityPeriodBar,
  ActivityTotals,
  TaskActivityTable,
  TruncatedNote,
} from "@/pages/activity/ActivityParts";
import { CostChart } from "@/pages/agentUsage/CostChart";
import { usageParams } from "@/pages/agentUsage/usageParams";
import TabPage from "@/components/TabPage";
import AgentStatusHistory from "./AgentStatusHistory";

// The URL keeps the older view names (?view=iterations|usage|status) so
// existing links still open the matching More view.
type MoreView = "iterations" | "usage" | "status";
const MORE_VIEWS: Array<{ value: MoreView; label: string }> = [
  { value: "iterations", label: "Runs log" },
  { value: "usage", label: "Usage" },
  { value: "status", label: "Status history" },
];

function moreViewFrom(params: URLSearchParams): MoreView {
  const raw = params.get("view");
  // Runs log is the default, and an ?iteration= deep link (e.g. from Usage)
  // opens it too.
  return raw === "usage" || raw === "status" ? raw : "iterations";
}

/** The agent's cost per bucket over the period, polled like the Usage view. */
function useCostSeries(name: string, since: string, until: string) {
  const api = useApi();
  const buildParams = useCallback(
    () =>
      usageParams({
        period: { since, until },
        preset: "24h",
        customSince: "",
        customUntil: "",
        groupBy: "model",
        now: Date.now(),
      }),
    [since, until],
  );
  const { data, refresh } = usePolling(() => api.usage.getAgentUsage(name, buildParams()), 10000);
  useEffect(() => {
    void refresh();
  }, [since, until, refresh]);
  return data?.series ?? [];
}

/** One Activity view for every agent: the period, its totals, the tasks worked
 *  on, the cost over time, and a More area with the runs log, usage and status
 *  history. */
export default function AgentActivityTab() {
  const name = useAgentName();
  const { window, setPeriod, setCustom } = useActivityWindow("today");
  const [searchParams, setSearchParams] = useSearchParams();
  const view = moreViewFrom(searchParams);
  const { report, error } = useTaskActivity(window, name);
  const [taskKey, setTaskKey] = useState("");
  // An invalid custom range shows its error and leaves the views unbounded.
  const bounds = { since: window.since, until: window.until };
  const series = useCostSeries(name, bounds.since, bounds.until);

  const selectView = (next: MoreView) => {
    const params = new URLSearchParams(searchParams);
    params.set("view", next);
    setSearchParams(params, { replace: true });
  };

  return (
    <TabPage className="overflow-auto">
      <ActivityPeriodBar
        window={window}
        periods={AGENT_ACTIVITY_PERIODS}
        onPeriod={setPeriod}
        onCustom={setCustom}
      />
      <ActivityTotals totals={report?.totals ?? null} variant="agent" />
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex flex-col gap-2">
        <TaskActivityTable rows={report?.tasks ?? []} onOpenTask={setTaskKey} />
        {report?.truncated && <TruncatedNote shown={report.tasks.length} />}
      </div>
      <CostChart series={series} />
      <Segmented label="More" options={MORE_VIEWS} value={view} onChange={selectView} />
      {view === "iterations" ? (
        <div className="h-[70vh] min-h-96 shrink-0">
          <AuditLogPage period={bounds} />
        </div>
      ) : view === "usage" ? (
        <AgentUsagePage period={bounds} />
      ) : (
        <AgentStatusHistory since={bounds.since} until={bounds.until} />
      )}
      {taskKey && <TaskDrawer key={taskKey} taskKey={taskKey} onClose={() => setTaskKey("")} />}
    </TabPage>
  );
}
