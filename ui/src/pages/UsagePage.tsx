import { useEffect, useRef, useState } from "react";
import { usePolling } from "@/hooks/usePolling";
import { getUsage, listGroups, type GroupRow } from "@/lib/api";
import { fmtTokens } from "@/lib/audit";
import { fmtDateTime } from "@/lib/time";
import type { UsagePoint, UsageReport } from "@/lib/types";

const UNGROUPED_FILTER = "__ungrouped__";
type TaggedUsage = { group: string; report: UsageReport };

function fmtCost(cost: number) {
  return `$${cost.toFixed(4)}`;
}

export default function UsagePage() {
  const [groups, setGroups] = useState<GroupRow[]>([]);
  const [selectedGroup, setSelectedGroup] = useState("");
  const selectedGroupRef = useRef(selectedGroup);
  const [acceptedUsage, setAcceptedUsage] = useState<TaggedUsage | null>(null);
  const { refresh } = usePolling(
    async () => {
      const group = selectedGroupRef.current;
      const report = await getUsage(group ? { group } : {});
      if (selectedGroupRef.current === group) setAcceptedUsage({ group, report });
    },
    5000,
  );

  useEffect(() => {
    void listGroups().then((result) => setGroups(result.groups)).catch(() => setGroups([]));
  }, []);
  useEffect(() => {
    void refresh();
  }, [selectedGroup, refresh]);

  const data = acceptedUsage?.group === selectedGroup ? acceptedUsage.report : null;
  const rows = data?.rows ?? [];
  const requests = data?.requests ?? [];
  return (
    <div className="space-y-4 p-6">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-lg font-semibold">Usage</h1>
        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Group</span>
          <select
            aria-label="Group"
            value={selectedGroup}
            onChange={(event) => {
              selectedGroupRef.current = event.target.value;
              setSelectedGroup(event.target.value);
            }}
            className="h-8 rounded border bg-background px-2"
          >
            <option value="">All groups</option>
            <option value={UNGROUPED_FILTER}>Ungrouped</option>
            {groups.map((group) => <option key={group.name} value={group.name}>{group.name}</option>)}
          </select>
        </label>
      </div>

      <dl aria-label="Usage summary" className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <SummaryCard label="Requests" value={data ? String(data.total_requests) : "—"} />
        <SummaryCard label="Input tokens" value={data ? fmtTokens(data.total_input_tokens) : "—"} />
        <SummaryCard label="Output tokens" value={data ? fmtTokens(data.total_output_tokens) : "—"} />
        <SummaryCard label="Historical cost" value={data ? fmtCost(data.total_cost_usd) : "—"} />
      </dl>

      <DailyUsage series={data?.series ?? []} />

      <section aria-labelledby="usage-aggregates-heading">
        <h2 id="usage-aggregates-heading" className="mb-2 text-base font-semibold">Usage by agent</h2>
        <div className="overflow-x-auto rounded border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2">Agent</th><th className="px-3 py-2">Image</th><th className="px-3 py-2">Group</th>
              <th className="px-3 py-2 text-right">Requests</th>
              <th className="px-3 py-2 text-right">In</th><th className="px-3 py-2 text-right">Out</th>
              <th className="px-3 py-2 text-right">Cache W</th><th className="px-3 py-2 text-right">Cache R</th>
              <th className="px-3 py-2 text-right">Cost</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={`${r.agent}:${r.image}:${r.group_id ?? ""}:${i}`} className="border-t">
                <td className="px-3 py-1.5">{r.agent}</td>
                <td className="px-3 py-1.5 text-muted-foreground">{r.image}</td>
                <td className="px-3 py-1.5">{r.group_name || "Ungrouped"}</td>
                <td className="px-3 py-1.5 text-right">{r.requests}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.input_tokens}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.output_tokens}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.cache_write_tokens}</td>
                <td className="px-3 py-1.5 text-right font-mono">{r.cache_read_tokens}</td>
                <td className="px-3 py-1.5 text-right font-mono">${r.cost_usd.toFixed(4)}</td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr><td colSpan={9} className="px-3 py-4 text-center text-muted-foreground">No usage yet.</td></tr>
            )}
          </tbody>
        </table>
        </div>
      </section>

      <section aria-labelledby="recent-requests-heading">
        <h2 id="recent-requests-heading" className="mb-2 text-base font-semibold">Recent requests</h2>
        <div className="overflow-x-auto rounded border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
              <tr>
                <th className="px-3 py-2">Timestamp</th><th className="px-3 py-2">Agent</th>
                <th className="px-3 py-2">Model</th><th className="px-3 py-2">Group</th>
                <th className="px-3 py-2 text-right">Input</th><th className="px-3 py-2 text-right">Output</th>
                <th className="px-3 py-2 text-right">Cache W</th><th className="px-3 py-2 text-right">Cache R</th>
                <th className="px-3 py-2 text-right">Cost</th>
              </tr>
            </thead>
            <tbody>
              {requests.map((request) => (
                <tr key={request.id} className="border-t">
                  <td className="px-3 py-1.5 whitespace-nowrap">{fmtDateTime(request.ts)}</td>
                  <td className="px-3 py-1.5">{request.agent}</td>
                  <td className="px-3 py-1.5 font-mono">{request.model}</td>
                  <td className="px-3 py-1.5">{request.group_name || "Ungrouped"}</td>
                  <td className="px-3 py-1.5 text-right font-mono">{request.input_tokens}</td>
                  <td className="px-3 py-1.5 text-right font-mono">{request.output_tokens}</td>
                  <td className="px-3 py-1.5 text-right font-mono">{request.cache_write_tokens}</td>
                  <td className="px-3 py-1.5 text-right font-mono">{request.cache_read_tokens}</td>
                  <td className="px-3 py-1.5 text-right font-mono">{fmtCost(request.cost_usd)}</td>
                </tr>
              ))}
              {requests.length === 0 && (
                <tr><td colSpan={9} className="px-3 py-4 text-center text-muted-foreground">No recent requests.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border p-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 font-mono text-lg">{value}</dd>
    </div>
  );
}

function DailyUsage({ series }: { series: UsagePoint[] }) {
  const max = series.reduce((highest, point) => Math.max(highest, point.cost_usd), 0);
  return (
    <section aria-label="Daily usage" className="rounded border p-3">
      <h2 className="text-base font-semibold">Daily usage</h2>
      {series.length === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">No daily usage yet.</p>
      ) : (
        <div className="mt-3 flex h-32 items-end gap-2" aria-label="Daily usage bars">
          {series.map((point) => {
            const detail = `${fmtDateTime(point.bucket_start)}: ${point.requests} requests, ${point.tokens} tokens, ${fmtCost(point.cost_usd)}`;
            const height = max > 0 ? `${Math.max((point.cost_usd / max) * 100, 2)}%` : "2%";
            return (
              <div key={point.bucket_start} className="flex min-w-0 flex-1 flex-col justify-end">
                <div className="rounded-t bg-primary" role="img" aria-label={detail} style={{ height }} />
                <span className="sr-only">{detail}</span>
                <span className="mt-1 truncate text-center text-xs text-muted-foreground">{fmtDateTime(point.bucket_start)}</span>
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}
