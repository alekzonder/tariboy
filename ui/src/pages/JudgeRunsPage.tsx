import { Link } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { usePolling } from "@/hooks/usePolling";
import { listJudgeRuns, type JudgeRun, type JudgeRunStatus } from "@/lib/judge";

const statusVariant = (status: JudgeRunStatus) => {
  if (status === "completed") return "default";
  if (status === "partial" || status === "cancelled") return "destructive";
  return "secondary";
};

// Kept exported for the focused formatting unit tests.
// eslint-disable-next-line react-refresh/only-export-components
export function compactCriteria(criteria: string | null | undefined): string {
  const oneLine = (criteria ?? "").replace(/\s+/g, " ").trim();
  return oneLine || "—";
}

function modelFor(run: JudgeRun): string {
  return run.model || (run.judge_agents ?? []).join(", ") || "—";
}

export default function JudgeRunsPage() {
  const { data, error } = usePolling(listJudgeRuns, 5000);
  const runs = data?.runs ?? [];

  return (
    <div className="space-y-4 p-6">
      <div>
        <h1 className="text-lg font-semibold">Judge runs</h1>
        <p className="text-sm text-muted-foreground">Historical iterations evaluated by LLM-as-Judge.</p>
      </div>
      {error && <p role="alert" className="rounded border border-destructive/40 px-3 py-2 text-sm text-destructive">Could not load judge runs: {error.message}</p>}
      <div className="overflow-x-auto rounded border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2">Criteria</th><th className="px-3 py-2">Count</th><th className="px-3 py-2">Coverage</th>
              <th className="px-3 py-2">Verdict</th><th className="px-3 py-2">Model</th><th className="px-3 py-2 text-right">Cost</th><th className="px-3 py-2">Creator</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr key={run.id} className="border-t">
                <td className="max-w-md px-3 py-2"><Link className="font-medium hover:underline" to={`/settings/advanced/judges/${encodeURIComponent(run.id)}`}>{compactCriteria(run.original_request)}</Link></td>
                <td className="px-3 py-2 font-mono whitespace-nowrap">{run.targets_ready}/{run.targets_total}</td>
                <td className="px-3 py-2 font-mono whitespace-nowrap">{run.assignments_completed}/{run.assignments_total}</td>
                <td className="px-3 py-2"><Badge variant={statusVariant(run.status)}>{run.status}</Badge></td>
                <td className="max-w-48 truncate px-3 py-2 text-muted-foreground" title={modelFor(run)}>{modelFor(run)}</td>
                <td className="px-3 py-2 text-right font-mono">{run.cost_usd === undefined ? "—" : `$${run.cost_usd.toFixed(4)}`}</td>
                <td className="px-3 py-2 text-muted-foreground">{run.lead_agent || run.creator_iteration || "—"}</td>
              </tr>
            ))}
            {!error && runs.length === 0 && <tr><td colSpan={7} className="px-3 py-4 text-center text-muted-foreground">No judge runs yet.</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
