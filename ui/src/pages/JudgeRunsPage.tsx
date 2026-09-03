import { useCallback, useEffect, useState } from "react";
import { Link, useOutletContext } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { usePolling } from "@/hooks/usePolling";
import { apiOn, resolveTarget, type ApiTarget } from "@/lib/api";
import { applyJudgeAutomation, getJudgeAutomation, listJudgeRunsOn, runJudgeAutomationOnce, validateJudgeAutomation, type JudgeAutomationDiagnostic, type JudgeRun, type JudgeRunStatus } from "@/lib/judge";

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
	const target = useOutletContext<ApiTarget>();
	const listRuns = useCallback(() => listJudgeRunsOn(target), [target]);
	const { data, error } = usePolling(listRuns, 5000);
	const runs = data?.runs ?? [];
	const [config, setConfig] = useState("");
	const [saved, setSaved] = useState("");
	const [diagnostics, setDiagnostics] = useState<JudgeAutomationDiagnostic[]>([]);
	const [configError, setConfigError] = useState("");
	const [busy, setBusy] = useState(false);
	const [judgesExists, setJudgesExists] = useState<boolean | null>(null);

	useEffect(() => {
		let current = true;
		void getJudgeAutomation(target).then((state) => {
			if (!current) return;
			const raw = state.revision?.canonical_json ?? "";
			setConfig(raw);
			setSaved(raw);
			return apiOn<{ groups: Array<{ name: string }> }>(resolveTarget(target), "GET", "/api/groups");
		}).then((result) => {
			if (current && result) setJudgesExists(result.groups.some((group) => group.name === "judges"));
		}).catch((cause) => { if (current) setConfigError((cause as Error).message); });
		return () => { current = false; };
	}, [target]);

	const validate = async () => {
		setBusy(true); setConfigError("");
		try { setDiagnostics((await validateJudgeAutomation(target, config)).diagnostics); }
		catch (cause) { setConfigError((cause as Error).message); }
		finally { setBusy(false); }
	};
	const apply = async () => {
		setBusy(true); setConfigError("");
		try {
			const result = await applyJudgeAutomation(target, config);
			setConfig(result.revision.canonical_json); setSaved(result.revision.canonical_json); setDiagnostics([]);
		} catch (cause) { setConfigError((cause as Error).message); }
		finally { setBusy(false); }
	};
	const runOnce = async () => {
		setBusy(true); setConfigError("");
		try { await runJudgeAutomationOnce(target); }
		catch (cause) { setConfigError((cause as Error).message); }
		finally { setBusy(false); }
	};
	const createJudges = async () => {
		setBusy(true); setConfigError("");
		try {
			const judge = (JSON.parse(saved) as { judge: { lead: string; workers: string[] } }).judge;
			await apiOn(resolveTarget(target), "POST", "/api/groups", { name: "judges", lead: judge.lead });
			for (const agent of [judge.lead, ...judge.workers]) await apiOn(resolveTarget(target), "POST", "/api/groups/judges/assign", { agent });
			setJudgesExists(true);
		} catch (cause) { setConfigError((cause as Error).message); }
		finally { setBusy(false); }
	};

  return (
    <div className="space-y-4 p-6">
      <div>
        <h1 className="text-lg font-semibold">Judge runs</h1>
        <p className="text-sm text-muted-foreground">Historical iterations evaluated by LLM-as-Judge.</p>
      </div>
	  <section className="space-y-3 rounded border p-4">
		<div><h2 className="font-medium">Automation configuration</h2><p className="text-sm text-muted-foreground">Raw JSON validated and applied by the selected tariboyd.</p></div>
		<label className="block space-y-1 text-sm"><span>Judge automation JSON</span><Textarea className="min-h-72 font-mono text-xs" value={config} onChange={(event) => setConfig(event.target.value)} spellCheck={false} /></label>
		{diagnostics.length > 0 && <ul role="alert" className="space-y-1 text-sm text-destructive">{diagnostics.map((item) => <li key={`${item.path}:${item.message}`}><code>{item.path}</code>: {item.message}</li>)}</ul>}
		{configError && <p className="text-sm text-destructive">{configError}</p>}
		<div className="flex gap-2"><Button type="button" variant="outline" disabled={busy} onClick={() => void validate()}>Validate</Button><Button type="button" disabled={busy} onClick={() => void apply()}>Apply</Button><Button type="button" variant="outline" disabled={busy} onClick={() => void runOnce()}>Run once</Button><Button type="button" variant="ghost" disabled={busy || config === saved} onClick={() => { setConfig(saved); setDiagnostics([]); }}>Reset</Button>{saved && judgesExists === false && <Button type="button" variant="outline" disabled={busy} onClick={() => void createJudges()}>Create judges team</Button>}</div>
	  </section>
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
