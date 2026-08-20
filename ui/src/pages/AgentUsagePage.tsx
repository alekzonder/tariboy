import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { usePolling } from "@/hooks/usePolling";
import { useAgentName } from "@/lib/agent";
import { getAgentUsage } from "@/lib/api";
import { fmtTokens } from "@/lib/audit";
import { fmtDateTime } from "@/lib/time";
import { cn } from "@/lib/utils";

// Window presets: label → (span in ms, series bucket). The bucket widens with
// the window so the chart never renders thousands of bars (spec §4).
const PRESETS = {
  "1h": { ms: 3600e3, bucket: "5m" },
  "5h": { ms: 5 * 3600e3, bucket: "15m" },
  "24h": { ms: 24 * 3600e3, bucket: "1h" },
  "7d": { ms: 7 * 24 * 3600e3, bucket: "1d" },
} as const;
type Preset = keyof typeof PRESETS | "custom";
const PRESET_ORDER: Preset[] = ["1h", "5h", "24h", "7d", "custom"];

const GROUPINGS: { key: "iteration" | "task" | "epic" | "model"; label: string }[] = [
  { key: "iteration", label: "Iterations" },
  { key: "task", label: "Tasks" },
  { key: "epic", label: "Epics" },
  { key: "model", label: "Models" },
];

// Pick the closest bucket to a custom span so the chart bar count stays sane.
function closestBucket(spanMs: number): string {
  if (spanMs <= 2 * 3600e3) return "5m";
  if (spanMs <= 8 * 3600e3) return "15m";
  if (spanMs <= 36 * 3600e3) return "1h";
  return "1d";
}

function fmtCost(usd: number): string {
  return `$${usd.toFixed(4)}`;
}

export default function AgentUsagePage() {
  const name = useAgentName();
  const [preset, setPreset] = useState<Preset>("24h");
  // datetime-local strings (local wall-clock), only used when preset==='custom'.
  const [customSince, setCustomSince] = useState("");
  const [customUntil, setCustomUntil] = useState("");
  const [groupBy, setGroupBy] = useState<"iteration" | "task" | "epic" | "model">("iteration");

  // Build the query params. Recomputed on every poll tick so preset windows
  // slide with the wall clock (since=now-Δ), and custom ranges convert their
  // local datetime-local values to RFC3339 (UTC) for the backend.
  const buildParams = useCallback((): Record<string, string> => {
    if (preset === "custom") {
      const since = customSince ? new Date(customSince).toISOString() : "";
      const until = customUntil ? new Date(customUntil).toISOString() : "";
      // Derive the bucket from the span so an open-ended custom start (from set,
      // to empty) still gets a coarse bucket via closestBucket instead of the
      // 1h default rendering hundreds of hourly bars (spec §4). Treat a missing
      // 'to' as now.
      let bucket = "1h";
      if (since) {
        const end = until ? new Date(until).getTime() : Date.now();
        bucket = closestBucket(end - new Date(since).getTime());
      }
      return { since, until, group_by: groupBy, bucket };
    }
    const { ms, bucket } = PRESETS[preset];
    return { since: new Date(Date.now() - ms).toISOString(), group_by: groupBy, bucket };
  }, [preset, customSince, customUntil, groupBy]);

  const { data, refresh } = usePolling(() => getAgentUsage(name, buildParams()), 10000);

  // Refetch immediately when the window or grouping changes, rather than waiting
  // for the next poll tick.
  useEffect(() => {
    void refresh();
  }, [preset, customSince, customUntil, groupBy, refresh]);

  const totals = data?.totals;
  const rows = data?.rows ?? [];
  const series = data?.series ?? [];

  return (
    <div className="flex flex-col gap-4">
      {/* (1) Window selector */}
      <div className="flex flex-wrap items-center gap-2">
        {PRESET_ORDER.map((p) => (
          <button
            key={p}
            onClick={() => setPreset(p)}
            className={cn(
              "rounded border px-3 py-1 text-sm hover:bg-accent",
              preset === p && "bg-accent font-medium",
            )}
          >
            {p === "custom" ? "Custom" : p}
          </button>
        ))}
        {preset === "custom" && (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <label className="flex items-center gap-1">
              <span className="text-muted-foreground">from</span>
              <input
                type="datetime-local"
                value={customSince}
                onChange={(e) => setCustomSince(e.target.value)}
                className="rounded border bg-background px-2 py-1"
              />
            </label>
            <label className="flex items-center gap-1">
              <span className="text-muted-foreground">to</span>
              <input
                type="datetime-local"
                value={customUntil}
                onChange={(e) => setCustomUntil(e.target.value)}
                className="rounded border bg-background px-2 py-1"
              />
            </label>
          </div>
        )}
      </div>

      {/* (2) Totals cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <TotalCard label="Requests" value={totals ? String(totals.requests) : "—"} />
        <TotalCard label="Tokens in" value={totals ? fmtTokens(totals.input_tokens) : "—"} />
        <TotalCard label="Tokens out" value={totals ? fmtTokens(totals.output_tokens) : "—"} />
        <TotalCard
          label="Cache w/r"
          value={totals ? `${fmtTokens(totals.cache_write_tokens)} / ${fmtTokens(totals.cache_read_tokens)}` : "—"}
        />
        <TotalCard label="Cost" value={totals ? fmtCost(totals.cost_usd) : "—"} />
      </div>

      {/* (3) Cost-over-time chart */}
      <CostChart series={series} />

      {/* (4) Grouping switch + table */}
      <div className="flex items-center gap-1">
        {GROUPINGS.map((g) => (
          <button
            key={g.key}
            onClick={() => setGroupBy(g.key)}
            className={cn(
              "rounded px-3 py-1 text-sm hover:bg-accent",
              groupBy === g.key ? "bg-accent font-medium" : "text-muted-foreground",
            )}
          >
            {g.label}
          </button>
        ))}
      </div>

      <div className="overflow-x-auto rounded border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2">{GROUPINGS.find((g) => g.key === groupBy)?.label ?? "Key"}</th>
              <th className="px-3 py-2 text-right">Requests</th>
              <th className="px-3 py-2 text-right">In</th>
              <th className="px-3 py-2 text-right">Out</th>
              <th className="px-3 py-2 text-right">Cache W</th>
              <th className="px-3 py-2 text-right">Cache R</th>
              <th className="px-3 py-2 text-right">Cost</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={i} className="border-t">
                <td className="px-3 py-1.5">
                  <RowKey name={name} groupBy={groupBy} rowKey={r.key} title={r.title} />
                </td>
                <td className="px-3 py-1.5 text-right">{r.requests}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.input_tokens}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.output_tokens}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.cache_write_tokens}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.cache_read_tokens}</td>
                <td className="px-3 py-1.5 text-right font-mono">{fmtCost(r.cost_usd)}</td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr>
                <td colSpan={7} className="px-3 py-4 text-center text-muted-foreground">
                  No usage in this window.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function TotalCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 font-mono text-lg">{value}</div>
    </div>
  );
}

// Iteration rows link to that iteration's Audit Log view; other groupings show
// the human title (task/epic) or the bare key (model), with 'untagged' plain.
function RowKey({
  name,
  groupBy,
  rowKey,
  title,
}: {
  name: string;
  groupBy: string;
  rowKey: string;
  title?: string;
}) {
  if (groupBy === "iteration") {
    if (rowKey === "untagged") return <span className="text-muted-foreground">untagged</span>;
    return (
      <Link
        to={`/agent/${encodeURIComponent(name)}/logs?iteration=${encodeURIComponent(rowKey)}`}
        className="font-mono text-xs text-primary hover:underline"
      >
        {rowKey}
      </Link>
    );
  }
  const label = title ?? rowKey;
  return <span className={cn(label === "untagged" && "text-muted-foreground")}>{label}</span>;
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
      <div className="flex h-[140px] items-center justify-center rounded border text-sm text-muted-foreground">
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
                  className="fill-muted-foreground text-[9px]"
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
