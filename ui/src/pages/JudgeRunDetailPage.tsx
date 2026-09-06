import { useCallback, useEffect, useState } from "react";
import { Link, useOutletContext, useParams, useSearchParams } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import type { ApiTarget } from "@/lib/api";
import { cancelJudgeRunOn, getJudgeEvidenceOn, getJudgeRunOn, retryJudgeRunOn, type JudgeAnalysis, type JudgeCitation, type JudgeRunDetail, type JudgeTarget } from "@/lib/judge";
import { hostToParam, paramToHost, serverPath, targetFor } from "@/lib/terminalsHost";

const terminal = new Set(["completed", "partial", "cancelled"]);
const statusVariant = (status: string) => status === "completed" ? "default" : status === "partial" || status === "cancelled" ? "destructive" : "secondary";
const money = (value?: number) => value === undefined ? "—" : `$${value.toFixed(4)}`;

function Citation({ target, runID, targetID, citation }: { target: ApiTarget; runID: string; targetID: string; citation: JudgeCitation }) {
  const [content, setContent] = useState<Record<string, unknown> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const load = async () => {
    setContent(null); setError(null);
    try { setContent((await getJudgeEvidenceOn(target, runID, targetID, citation.artifact, citation.locator)).evidence); }
    catch (cause) { setError((cause as Error).message); }
  };
  return <span className="inline-flex items-center gap-1"><Button variant="link" className="h-auto p-0 text-xs" onClick={() => void load()}>[{citation.artifact}:{citation.locator}]</Button>{content && <span className="max-w-lg rounded bg-muted px-2 py-1 text-xs"><span className="font-medium">Immutable evidence (untrusted): </span>{JSON.stringify(content)}</span>}{error && <span className="text-xs text-destructive">Evidence unavailable: {error}</span>}</span>;
}

function Analysis({ target, runID, targetID, analysis }: { target: ApiTarget; runID: string; targetID: string; analysis: JudgeAnalysis }) {
  return <div className="mt-3 border-t pt-3 text-sm">
    <p><b>{analysis.judge_agent}</b>: {analysis.result.verdict} ({analysis.result.score.toFixed(2)}, confidence {analysis.result.confidence.toFixed(2)})</p>
    <p className="text-muted-foreground">{analysis.result.summary}</p>
    {[...(analysis.result.violations || []), ...(analysis.result.strengths || [])].map((finding, index) => <p className="mt-1" key={index}>{finding.severity && <Badge variant="secondary">{finding.severity}</Badge>} {finding.description} {finding.citations?.map((citation, citationIndex) => <Citation key={citationIndex} target={target} runID={runID} targetID={targetID} citation={citation} />)}</p>)}
    {analysis.result.evidence_gaps?.length ? <div className="mt-2"><b>Evidence gaps</b><ul className="list-disc pl-5">{analysis.result.evidence_gaps.map(gap => <li key={gap}>{gap}</li>)}</ul></div> : null}
  </div>;
}

function TargetAnalysis({ apiTarget, runID, judgeTarget, analyses, targetOnly = false }: { apiTarget: ApiTarget; runID: string; judgeTarget: JudgeTarget; analyses: JudgeAnalysis[]; targetOnly?: boolean }) {
  return <div className="rounded border p-3">
    <div className="flex flex-wrap justify-between gap-2 text-sm"><span><b>{targetOnly ? judgeTarget.agent : `${judgeTarget.sequence + 1}. ${judgeTarget.agent}`}</b> <span className="font-mono text-xs">{judgeTarget.iteration}</span></span><span>{judgeTarget.assignments_completed} completed · {judgeTarget.assignments_failed} failed · {judgeTarget.assignments_pending} pending · <Badge variant={statusVariant(judgeTarget.target_state)}>{judgeTarget.consensus_verdict || judgeTarget.target_state}</Badge></span></div>
    {targetOnly && <p className="mt-2 text-sm">Consensus score {judgeTarget.consensus_score === undefined ? "—" : judgeTarget.consensus_score.toFixed(2)}</p>}
    {analyses.map(analysis => <Analysis key={analysis.id} target={apiTarget} runID={runID} targetID={judgeTarget.id} analysis={analysis} />)}
  </div>;
}

export default function JudgeRunDetailPage() {
  const { id = "", hostId: hostParam = "local" } = useParams();
  const [searchParams] = useSearchParams();
  const contextTarget = useOutletContext<ApiTarget>();
  const hostId = paramToHost(hostParam);
  const apiTarget = contextTarget === undefined ? targetFor(hostId) : contextTarget;
  const selectedTarget = searchParams.get("target");
  const descriptor = apiTarget && `${apiTarget.id}\0${apiTarget.baseURL}\0${apiTarget.token}`;
  return <RunDetailView key={`${hostId}\0${id}\0${selectedTarget ?? ""}\0${descriptor ?? "local"}`} id={id} hostId={hostId} selectedTarget={selectedTarget} target={apiTarget} />;
}

function RunDetailView({ id, hostId, selectedTarget, target }: { id: string; hostId: string; selectedTarget: string | null; target: ApiTarget }) {
  const [data, setData] = useState<JudgeRunDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<"retry" | "cancel" | null>(null);
  const [busy, setBusy] = useState(false);
  const fetchRun = useCallback(() => getJudgeRunOn(target, id), [id, target]);
  const load = useCallback(async () => { try { setError(null); setData(await fetchRun()); } catch (cause) { setError((cause as Error).message); } }, [fetchRun]);
  useEffect(() => { let current = true; void fetchRun().then(result => { if (current) setData(result); }).catch(cause => { if (current) setError((cause as Error).message); }); return () => { current = false; }; }, [fetchRun]);
  useEffect(() => { if (!data || terminal.has(data.run.status)) return; let current = true; const timer = window.setInterval(() => { void fetchRun().then(result => { if (current) setData(result); }).catch(cause => { if (current) setError((cause as Error).message); }); }, 5000); return () => { current = false; window.clearInterval(timer); }; }, [data, fetchRun]);
  const act = async () => { if (!confirm) return; setBusy(true); try { if (confirm === "retry") await retryJudgeRunOn(target, id); else await cancelJudgeRunOn(target, id); setConfirm(null); await load(); } catch (cause) { setError((cause as Error).message); } finally { setBusy(false); } };
  const judgesPath = `${serverPath(hostId, "settings")}/advanced/judges`;
  if (error && !data) return <div className="space-y-4 p-6"><Link className="text-sm underline" to={judgesPath}>Judge runs</Link><p role="alert" className="text-destructive">Could not load judge run: {error}</p></div>;
  if (!data) return <div className="p-6 text-muted-foreground">Loading judge run…</div>;
  const { run, targets, analyses, summaries, usage, improvements = [] } = data;
  const chosen = selectedTarget === null ? null : targets.find(item => item.id === selectedTarget);
  if (selectedTarget !== null && !chosen) return <div className="space-y-4 p-6"><Link className="text-sm underline" to={judgesPath}>Judge runs</Link><p role="alert" className="text-destructive">Target {selectedTarget} was not found in judge run {run.id}.</p></div>;
  if (chosen) return <div className="space-y-5 p-6">
    <nav aria-label="Breadcrumb" className="flex gap-3 text-sm"><Link className="underline" to={judgesPath}>Judge runs</Link><Link className="underline" to={`/agents/${encodeURIComponent(hostToParam(hostId))}/${encodeURIComponent(chosen.agent)}/activity?iteration=${encodeURIComponent(chosen.iteration)}`}>{chosen.agent} iteration {chosen.iteration}</Link></nav>
    <div><h1 className="text-lg font-semibold">Judge analysis for <span className="font-mono text-base">{chosen.iteration}</span></h1><p className="text-sm text-muted-foreground">Run <span className="font-mono">{run.id}</span></p></div>
    <section><h2 className="mb-2 font-semibold">Target consensus and analyses</h2><TargetAnalysis apiTarget={target} runID={run.id} judgeTarget={chosen} analyses={analyses.filter(item => item.target_id === chosen.id)} targetOnly /></section>
  </div>;
  return <div className="space-y-5 p-6">
    <div className="flex items-start justify-between gap-4"><div><Link className="text-sm underline" to={judgesPath}>← Judge runs</Link><h1 className="mt-2 text-lg font-semibold">Judge run <span className="font-mono text-base">{run.id}</span></h1><p className="mt-1 max-w-3xl whitespace-pre-wrap text-sm text-muted-foreground">{run.original_request}</p></div><div className="flex gap-2"><Badge variant={statusVariant(run.status)}>{run.status}</Badge>{run.status === "partial" && <Button onClick={() => setConfirm("retry")}>Retry failed work</Button>}{!terminal.has(run.status) && <Button variant="destructive" onClick={() => setConfirm("cancel")}>Cancel run</Button>}</div></div>
    {error && <p role="alert" className="text-sm text-destructive">Action failed: {error}</p>}
    <section className="grid gap-3 rounded border p-4 text-sm md:grid-cols-3"><div><b>Spec</b><br />{run.judge_group}; {run.judges_per_iteration} judge(s)/target; max {run.max_attempts} attempts</div><div><b>Manifest</b><br /><span className="font-mono text-xs">{run.manifest_hash || "—"}</span></div><div><b>Progress</b><br />{run.targets_ready}/{run.targets_total} targets · {run.assignments_completed}/{run.assignments_total} assignments · summary v{run.current_summary_version}</div><div><b>Models</b><br />{run.model || (run.judge_agents ?? []).join(", ") || "—"}</div><div><b>Cost</b><br />{money(run.cost_usd)}</div><div><b>Last error</b><br />{run.last_error || "None"}</div></section>
    <section><h2 className="mb-2 font-semibold">Targets and analyses</h2><div className="space-y-3">{targets.map(item => <TargetAnalysis key={item.id} apiTarget={target} runID={run.id} judgeTarget={item} analyses={analyses.filter(analysis => analysis.target_id === item.id)} />)}</div></section>
    <section><h2 className="mb-2 font-semibold">Summary versions</h2>{summaries.length ? summaries.map(summary => <div className="mb-2 rounded border p-3 text-sm" key={summary.id}><b>Version {summary.version}</b> · {summary.summary_agent}<p className="mt-1">{summary.result.executive_conclusion || "No executive conclusion."}</p>{summary.result.recommendations?.length ? <p className="mt-1 text-muted-foreground">Recommendations: {summary.result.recommendations.join("; ")}</p> : null}</div>) : <p className="text-sm text-muted-foreground">No summary version yet.</p>}</section>
    <section><h2 className="mb-2 font-semibold">Improvement proposals</h2>{improvements.length ? improvements.map(proposal => <div className="mb-2 rounded border p-3 text-sm" key={proposal.id}><Link className="font-mono underline" to={`${serverPath(hostId, "settings")}/advanced/improvements/${encodeURIComponent(proposal.id)}`}>{proposal.id}</Link> · {proposal.status}<p>{proposal.draft.target.repository} @ {proposal.draft.target.base_commit}</p></div>) : <p className="text-sm text-muted-foreground">No improvement proposal.</p>}</section>
    <section><h2 className="mb-2 font-semibold">Target usage</h2><div className="overflow-x-auto rounded border"><table className="w-full text-sm"><thead className="bg-muted/50 text-left"><tr><th className="p-2">Iteration</th><th className="p-2">Requests</th><th className="p-2">Input</th><th className="p-2">Output</th><th className="p-2 text-right">Cost</th></tr></thead><tbody>{usage.map(item => <tr className="border-t" key={item.iteration}><td className="p-2 font-mono text-xs">{item.iteration}</td><td className="p-2">{item.requests}</td><td className="p-2">{item.input_tokens}</td><td className="p-2">{item.output_tokens}</td><td className="p-2 text-right">{money(item.cost_usd)}</td></tr>)}</tbody></table></div></section>
    <AlertDialog open={!!confirm} onOpenChange={(open) => !open && setConfirm(null)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{confirm === "retry" ? "Retry failed assignments?" : "Cancel this judge run?"}</AlertDialogTitle><AlertDialogDescription>{confirm === "retry" ? "This creates new work only for eligible failed assignments." : "Pending work will be cancelled; existing immutable evidence stays available."}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={busy}>Keep run</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(event) => { event.preventDefault(); void act(); }}>{busy ? "Working…" : confirm === "retry" ? "Retry" : "Cancel run"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>;
}
