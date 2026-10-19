import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import type { ApiTarget } from "@/lib/api";
import { periodQuery, useActivityWindow } from "@/lib/activityPeriod";
import { hostToParam } from "@/lib/terminalsHost";
import { useTaskActivity } from "@/hooks/useTaskActivity";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import {
  ActivityPeriodBar,
  ActivityTabs,
  ActivityTotals,
  AgentActivityTable,
  TaskActivityTable,
  TruncatedNote,
} from "./ActivityParts";
import TabPage from "@/components/TabPage";

type View = "tasks" | "agents";
const VIEWS: Array<{ view: View; label: string }> = [
  { view: "tasks", label: "Tasks" },
  { view: "agents", label: "Agents" },
];

/** Work on tasks across every agent of one server in the selected period. */
export default function ServerActivityPage({ hostId, target }: { hostId: string; target: ApiTarget }) {
  const { window, setPeriod, setCustom } = useActivityWindow();
  const [searchParams, setSearchParams] = useSearchParams();
  const view: View = searchParams.get("view") === "agents" ? "agents" : "tasks";
  const { report, error } = useTaskActivity(target, window);
  const [taskKey, setTaskKey] = useState("");
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
      {view === "tasks" ? (
        <>
          <TaskActivityTable
            rows={report?.tasks ?? []}
            showAgents
            onOpenTask={setTaskKey}
          />
          {report?.truncated && <TruncatedNote shown={report.tasks.length} />}
        </>
      ) : (
        <AgentActivityTable
          agents={report?.agents ?? []}
          agentHref={(agent) =>
            `/agents/${encodeURIComponent(hostToParam(hostId))}/${encodeURIComponent(agent)}/activity?${periodQuery(window)}`}
        />
      )}
      {taskKey && <TaskDrawer key={taskKey} taskKey={taskKey} target={target} onClose={() => setTaskKey("")} />}
    </TabPage>
  );
}
