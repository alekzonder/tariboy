/* eslint-disable react-hooks/set-state-in-effect */
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { cancelJudgeRun, getJudgeEvidence, getJudgeRun, retryJudgeRun, type JudgeCitation, type JudgeRunDetail } from "@/lib/judge";

const terminal = new Set(["completed", "partial", "cancelled"]);
const statusVariant = (status: string) => status === "completed" ? "default" : status === "partial" || status === "cancelled" ? "destructive" : "secondary";
const money = (value?: number) => value === undefined ? "—" : `$${value.toFixed(4)}`;

function Citation({ runID, targetID, citation }: { runID: string; targetID: string; citation: JudgeCitation }) {
  const [content, setContent] = useState<Record<string, unknown> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const load = async () => { try { setError(null); setContent((await getJudgeEvidence(runID, targetID, citation.artifact, citation.locator)).evidence); } catch (e) { setError((e as Error).message); } };
  return <span className="inline-flex items-center gap-1"><Button variant="link" className="h-auto p-0 text-xs" onClick={() => void load()}>[{citation.artifact}:{citation.locator}]</Button>{content && <span className="max-w-lg rounded bg-muted px-2 py-1 text-xs"><span className="font-medium">Immutable evidence (untrusted): </span>{JSON.stringify(content)}</span>}{error && <span className="text-xs text-destructive">Evidence unavailable: {error}</span>}</span>;
}

export default function JudgeRunDetailPage() {
  const { id = "" } = useParams();
  const [data, setData] = useState<JudgeRunDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<"retry" | "cancel" | null>(null);
  const [busy, setBusy] = useState(false);
  const load = async () => { try { setError(null); setData(await getJudgeRun(id)); } catch (e) { setError((e as Error).message); } };
  useEffect(() => { void load(); }, [id]);
  useEffect(() => { if (!data || terminal.has(data.run.status)) return; const timer = window.setInterval(() => void load(), 5000); return () => window.clearInterval(timer); }, [data?.run.status, id]); // polling intentionally ends at terminal states
  const act = async () => { if (!confirm) return; setBusy(true); try { if (confirm === "retry") await retryJudgeRun(id); else await cancelJudgeRun(id); setConfirm(null); await load(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } };
  if (error && !data) return <div className="space-y-4 p-6"><Link className="text-sm underline" to="/settings/advanced/judges">Judge runs</Link><p role="alert" className="text-destructive">Could not load judge run: {error}</p></div>;
  if (!data) return <div className="p-6 text-muted-foreground">Loading judge run…</div>;
  const { run, targets, analyses, summaries, usage } = data;
  return <div className="space-y-5 p-6">
    <div className="flex items-start justify-between gap-4"><div><Link className="text-sm underline" to="/settings/advanced/judges">← Judge runs</Link><h1 className="mt-2 text-lg font-semibold">Judge run <span className="font-mono text-base">{run.id}</span></h1><p className="mt-1 max-w-3xl whitespace-pre-wrap text-sm text-muted-foreground">{run.original_request}</p></div><div className="flex gap-2"><Badge variant={statusVariant(run.status)}>{run.status}</Badge>{run.status === "partial" && <Button onClick={() => setConfirm("retry")}>Retry failed work</Button>}{!terminal.has(run.status) && <Button variant="destructive" onClick={() => setConfirm("cancel")}>Cancel run</Button>}</div></div>
    {error && <p role="alert" className="text-sm text-destructive">Action failed: {error}</p>}
    <section className="grid gap-3 rounded border p-4 text-sm md:grid-cols-3"><div><b>Spec</b><br />{run.judge_group}; {run.judges_per_iteration} judge(s)/target; max {run.max_attempts} attempts</div><div><b>Manifest</b><br /><span className="font-mono text-xs">{run.manifest_hash || "—"}</span></div><div><b>Progress</b><br />{run.targets_ready}/{run.targets_total} targets · {run.assignments_completed}/{run.assignments_total} assignments · summary v{run.current_summary_version}</div><div><b>Models</b><br />{run.model || (run.judge_agents ?? []).join(", ") || "—"}</div><div><b>Cost</b><br />{money(run.cost_usd)}</div><div><b>Last error</b><br />{run.last_error || "None"}</div></section>
    <section><h2 className="mb-2 font-semibold">Targets and analyses</h2><div className="space-y-3">{targets.map(t => <div className="rounded border p-3" key={t.id}><div className="flex flex-wrap justify-between gap-2 text-sm"><span><b>{t.sequence + 1}. {t.agent}</b> <span className="font-mono text-xs">{t.iteration}</span></span><span>{t.assignments_completed} completed · {t.assignments_failed} failed · {t.assignments_pending} pending · <Badge variant={statusVariant(t.target_state)}>{t.consensus_verdict || t.target_state}</Badge></span></div>{analyses.filter(a => a.target_id === t.id).map(a => <div className="mt-3 border-t pt-3 text-sm" key={a.id}><p><b>{a.judge_agent}</b>: {a.result.verdict} ({a.result.score.toFixed(2)}, confidence {a.result.confidence.toFixed(2)})</p><p className="text-muted-foreground">{a.result.summary}</p>{[...(a.result.violations || []), ...(a.result.strengths || [])].map((f, i) => <p className="mt-1" key={i}>{f.severity && <Badge variant="secondary">{f.severity}</Badge>} {f.description} {f.citations?.map((c, j) => <Citation key={j} runID={run.id} targetID={t.id} citation={c} />)}</p>)}</div>)}</div>)}</div></section>
    <section><h2 className="mb-2 font-semibold">Summary versions</h2>{summaries.length ? summaries.map(s => <div className="mb-2 rounded border p-3 text-sm" key={s.id}><b>Version {s.version}</b> · {s.summary_agent}<p className="mt-1">{s.result.executive_conclusion || "No executive conclusion."}</p>{s.result.recommendations?.length ? <p className="mt-1 text-muted-foreground">Recommendations: {s.result.recommendations.join("; ")}</p> : null}</div>) : <p className="text-sm text-muted-foreground">No summary version yet.</p>}</section>
    <section><h2 className="mb-2 font-semibold">Target usage</h2><div className="overflow-x-auto rounded border"><table className="w-full text-sm"><thead className="bg-muted/50 text-left"><tr><th className="p-2">Iteration</th><th className="p-2">Requests</th><th className="p-2">Input</th><th className="p-2">Output</th><th className="p-2 text-right">Cost</th></tr></thead><tbody>{usage.map(u => <tr className="border-t" key={u.iteration}><td className="p-2 font-mono text-xs">{u.iteration}</td><td className="p-2">{u.requests}</td><td className="p-2">{u.input_tokens}</td><td className="p-2">{u.output_tokens}</td><td className="p-2 text-right">{money(u.cost_usd)}</td></tr>)}</tbody></table></div></section>
    <AlertDialog open={!!confirm} onOpenChange={(open) => !open && setConfirm(null)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{confirm === "retry" ? "Retry failed assignments?" : "Cancel this judge run?"}</AlertDialogTitle><AlertDialogDescription>{confirm === "retry" ? "This creates new work only for eligible failed assignments." : "Pending work will be cancelled; existing immutable evidence stays available."}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={busy}>Keep run</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={(e) => { e.preventDefault(); void act(); }}>{busy ? "Working…" : confirm === "retry" ? "Retry" : "Cancel run"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>;
}
