import type { UsageSeriesPoint } from "@/lib/api";
import { fmtDateTime } from "@/lib/time";
import { fmtCost } from "@/lib/usage";

// A dependency-free SVG bar chart of cost per time bucket. Bars scale to the
// max bucket cost; each carries a <title> tooltip with the bucket + exact cost.
export function CostChart({ series }: { series: UsageSeriesPoint[] }) {
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
