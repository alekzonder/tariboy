import { useCallback, useEffect, useState } from "react";
import { usePolling } from "@/hooks/usePolling";
import { useAgentName } from "@/lib/agent";
import { useApi } from "@/components/apiContext";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import { CostChart } from "@/pages/agentUsage/CostChart";
import { UsageGroupBySwitch, UsageWindowPicker } from "@/pages/agentUsage/UsageWindowPicker";
import { UsageTable, UsageTotals } from "@/pages/agentUsage/UsageTable";
import { usageParams, type GroupBy, type Preset } from "@/pages/agentUsage/usageParams";

// period, when given, is the Activity tab's shared window: it replaces the
// page's own window presets, and the tab draws the cost chart itself.
export default function AgentUsagePage({
  period,
}: { period?: { since: string; until: string } } = {}) {
  const api = useApi();
  const name = useAgentName();
  const [preset, setPreset] = useState<Preset>("24h");
  // datetime-local strings (local wall-clock), only used when preset==='custom'.
  const [customSince, setCustomSince] = useState("");
  const [customUntil, setCustomUntil] = useState("");
  const [groupBy, setGroupBy] = useState<GroupBy>("iteration");
  const [taskKey, setTaskKey] = useState("");

  // Recomputed on every poll tick so preset windows slide with the wall clock.
  const managed = period !== undefined;
  const periodSince = period?.since ?? "";
  const periodUntil = period?.until ?? "";
  const buildParams = useCallback(
    () =>
      usageParams({
        period: managed ? { since: periodSince, until: periodUntil } : undefined,
        preset,
        customSince,
        customUntil,
        groupBy,
        now: Date.now(),
      }),
    [managed, periodSince, periodUntil, preset, customSince, customUntil, groupBy],
  );

  const { data, refresh } = usePolling(() => api.usage.getAgentUsage(name, buildParams()), 10000);

  // Refetch immediately when the window or grouping changes, rather than waiting
  // for the next poll tick.
  useEffect(() => {
    void refresh();
  }, [periodSince, periodUntil, preset, customSince, customUntil, groupBy, refresh]);

  return (
    <div className="flex flex-col gap-4">
      {/* (1) Window selector */}
      {!period && (
        <UsageWindowPicker
          preset={preset}
          onPreset={setPreset}
          customSince={customSince}
          onCustomSince={setCustomSince}
          customUntil={customUntil}
          onCustomUntil={setCustomUntil}
        />
      )}

      {/* (2) Totals cards */}
      <UsageTotals totals={data?.totals} />

      {/* (3) Cost-over-time chart; the Activity tab draws its own above. */}
      {!period && <CostChart series={data?.series ?? []} />}

      {/* (4) Grouping switch + table */}
      <UsageGroupBySwitch groupBy={groupBy} onGroupBy={setGroupBy} />
      <UsageTable name={name} groupBy={groupBy} rows={data?.rows ?? []} onOpenTask={setTaskKey} />
      {taskKey && <TaskDrawer key={taskKey} taskKey={taskKey} onClose={() => setTaskKey("")} />}
    </div>
  );
}
