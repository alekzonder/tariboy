import { useCallback, useEffect, useState } from "react";
import { usePolling } from "@/hooks/usePolling";
import { useAgentName } from "@/lib/agent";
import { fmtDateTime } from "@/lib/time";
import { fmtCost } from "@/lib/usage";
import { useApi } from "@/components/apiContext";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import { UsageGroupBySwitch, UsageWindowPicker } from "@/pages/agentUsage/UsageWindowPicker";
import { UsageTable, UsageTotals } from "@/pages/agentUsage/UsageTable";
import { usageParams, type GroupBy, type Preset } from "@/pages/agentUsage/usageParams";

// period, when given, is the Activity tab's shared window: it replaces the
// page's own window presets.
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

      {/* (3) Cost-over-time chart */}
      <CostChart series={data?.series ?? []} />

      {/* (4) Grouping switch + table */}
      <UsageGroupBySwitch groupBy={groupBy} onGroupBy={setGroupBy} />
      <UsageTable name={name} groupBy={groupBy} rows={data?.rows ?? []} onOpenTask={setTaskKey} />
      {taskKey && <TaskDrawer key={taskKey} taskKey={taskKey} onClose={() => setTaskKey("")} />}
    </div>
  );
}

// A dependency-free SVG bar chart of cost per time bucket. Bars scale to the
// max bucket cost; each carries a <title> tooltip with the bucket + exact cost.
function CostChart({ series }: { series: { bucket_start: string; cost_usd: number }[] }) {
  const W = 800;
  const H = 140;
  const pad = { top: 8, right: 8, bottom: 20, left: 8 };
  const plotW = W - pad.left - pad.right;
  const plotH = H - pad.top - pad.bottom;
  const max = series.reduce((m, p) => Math.max(m, p.cost_usd), 0);

  if (series.length === 0 || max === 0) {
    return (
      <div className="flex h-35 items-center justify-center rounded border text-sm text-muted-foreground">
        No cost in this window.
      </div>
    );
  }

  const n = series.length;
  const slot = plotW / n;
  const barW = Math.max(1, slot * 0.7);
  // Label at most ~6 x-ticks so they never overlap.
  const tickEvery = Math.max(1, Math.ceil(n / 6));

  return (
    <div className="rounded border p-2">
      <div className="mb-1 text-xs text-muted-foreground">Cost over time (max {fmtCost(max)})</div>
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full" role="img" aria-label="cost over time">
        {series.map((p, i) => {
          const h = (p.cost_usd / max) * plotH;
          const x = pad.left + i * slot + (slot - barW) / 2;
          const y = pad.top + (plotH - h);
          return (
            <g key={i}>
              <rect x={x} y={y} width={barW} height={h} className="fill-primary" rx={1}>
                <title>{`${fmtDateTime(p.bucket_start)} — ${fmtCost(p.cost_usd)}`}</title>
              </rect>
              {i % tickEvery === 0 && (
                <text
                  x={pad.left + i * slot + slot / 2}
                  y={H - 6}
                  textAnchor="middle"
                  className="fill-muted-foreground text-nano"
                >
                  {fmtDateTime(p.bucket_start)}
                </text>
              )}
            </g>
          );
        })}
      </svg>
    </div>
  );
}
