import { Link } from "react-router-dom";
import { fmtTokens } from "@/lib/audit";
import type { AgentUsage } from "@/lib/api";
import { fmtCost } from "@/lib/usage";
import { cn } from "@/lib/utils";
import UsageSummaryCard from "@/components/usage/UsageSummaryCard";
import { GROUPINGS, type GroupBy } from "./usageParams";

export function UsageTotals({ totals }: { totals?: AgentUsage["totals"] }) {
  return (
    <dl aria-label="Usage totals" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      <UsageSummaryCard label="Requests" value={totals ? String(totals.requests) : "—"} />
      <UsageSummaryCard label="Tokens in" value={totals ? fmtTokens(totals.input_tokens) : "—"} />
      <UsageSummaryCard label="Tokens out" value={totals ? fmtTokens(totals.output_tokens) : "—"} />
      <UsageSummaryCard
        label="Cache w/r"
        value={
          totals
            ? `${fmtTokens(totals.cache_write_tokens)} / ${fmtTokens(totals.cache_read_tokens)}`
            : "—"
        }
      />
      <UsageSummaryCard label="Cost" value={totals ? fmtCost(totals.cost_usd) : "—"} />
    </dl>
  );
}

export function UsageTable({
  name,
  groupBy,
  rows,
  onOpenTask,
}: {
  name: string;
  groupBy: GroupBy;
  rows: AgentUsage["rows"];
  onOpenTask: (key: string) => void;
}) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2">
              {GROUPINGS.find((g) => g.key === groupBy)?.label ?? "Key"}
            </th>
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
                <RowKey
                  name={name}
                  groupBy={groupBy}
                  rowKey={r.key}
                  title={r.title}
                  onOpenTask={onOpenTask}
                />
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
  );
}

// Iteration rows link to that iteration's Audit Log view; task/epic rows open
// the task key in place beside its title; model rows show the bare key, with
// 'untagged' plain.
function RowKey({
  name,
  groupBy,
  rowKey,
  title,
  onOpenTask,
}: {
  name: string;
  groupBy: string;
  rowKey: string;
  title?: string;
  onOpenTask: (key: string) => void;
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
  if ((groupBy === "task" || groupBy === "epic") && rowKey !== "untagged") {
    return (
      <span className="flex flex-wrap items-baseline gap-2">
        <button
          type="button"
          onClick={() => onOpenTask(rowKey)}
          className="font-mono text-xs text-primary hover:underline"
        >
          {rowKey}
        </button>
        {title && title !== rowKey && <span>{title}</span>}
      </span>
    );
  }
  const label = title ?? rowKey;
  return <span className={cn(label === "untagged" && "text-muted-foreground")}>{label}</span>;
}
