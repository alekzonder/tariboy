/* eslint-disable react-hooks/set-state-in-effect */
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { decideImprovementPlan, decideReleaseRollout, getImprovement, stageReleaseRollout, type ImprovementDetail } from "@/lib/improvement";

export default function ImprovementDetailPage() {
  const { id = "" } = useParams();
  const [detail, setDetail] = useState<ImprovementDetail | null>(null);
  const [agent, setAgent] = useState("");
  const [error, setError] = useState("");
  const load = async () => { try { setError(""); setDetail(await getImprovement(id)); } catch (cause) { setError((cause as Error).message); } };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void load(); }, [id]);
  if (!detail) return <div className="p-6">{error || "Loading improvement…"}</div>;
  const { proposal, releases } = detail;
  const decidePlan = async (decision: "approve" | "reject") => { try { await decideImprovementPlan(proposal.id, decision, proposal.revision_hash); await load(); } catch (cause) { setError((cause as Error).message); } };
  return <div className="space-y-5 p-6">
    <div><Link className="text-sm underline" to={`/settings/advanced/judges/${encodeURIComponent(proposal.judge_run_id)}`}>← Judge run</Link><h1 className="mt-2 text-lg font-semibold">Improvement <span className="font-mono text-base">{proposal.id}</span></h1><Badge variant="secondary">{proposal.status}</Badge></div>
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    <section className="grid gap-3 rounded border p-4 text-sm md:grid-cols-2"><div><b>Repository / base commit</b><br />{proposal.draft.target.repository} · <span className="font-mono text-xs">{proposal.draft.target.base_commit}</span></div><div><b>Current image</b><br />{proposal.draft.target.image} · <span className="font-mono text-xs">{proposal.draft.target.image_digest}</span></div><div><b>Risk</b><br />{proposal.draft.risk}</div><div><b>Rollback</b><br />{proposal.draft.rollback_image}</div><div className="md:col-span-2"><b>Exact revision</b><br /><span className="break-all font-mono text-xs">{proposal.revision_hash}</span></div></section>
    <section><h2 className="font-semibold">Evidence-linked findings</h2>{proposal.draft.findings.map((finding, index) => <div className="mt-2 rounded border p-3 text-sm" key={index}><b>{finding.severity}: {finding.criterion}</b><p>{finding.observation}</p>{finding.evidence.map((citation, i) => <p className="break-all font-mono text-xs text-muted-foreground" key={i}>{citation.bundle_hash} · {citation.artifact}:{citation.locator}</p>)}</div>)}</section>
    <section><h2 className="font-semibold">Approved file scope</h2>{proposal.draft.changes.map(change => <div className="mt-2 rounded border p-3 text-sm" key={change.file}><span className="font-mono">{change.file}</span><p className="text-muted-foreground">{change.intent}</p></div>)}</section>
    <section><h2 className="font-semibold">Acceptance criteria</h2><ul className="list-disc pl-5 text-sm">{proposal.draft.acceptance.map(item => <li key={item}>{item}</li>)}</ul></section>
    {proposal.status === "awaiting_plan_approval" && <div className="flex gap-2"><Button onClick={() => void decidePlan("approve")}>Approve exact plan</Button><Button variant="destructive" onClick={() => void decidePlan("reject")}>Reject plan</Button></div>}
    {releases.map(release => <section className="rounded border p-4 text-sm" key={release.id}><h2 className="font-semibold">Release {release.image_ref}</h2><p className="break-all font-mono text-xs">{release.image_digest}</p><p>Commit {release.git_commit} · source {release.source_digest} · lock {release.lock_digest} · prompt {release.prompt_template_digest}</p><p className="break-all">Approval hash: <span className="font-mono text-xs">{release.release_hash}</span></p>{release.status === "image_built" && <div className="mt-3 flex gap-2"><Button onClick={async () => { await decideReleaseRollout(release.id, "approve", release.release_hash); await load(); }}>Approve rollout</Button><Button variant="destructive" onClick={async () => { await decideReleaseRollout(release.id, "reject", release.release_hash); await load(); }}>Reject rollout</Button></div>}<div className="mt-3 flex gap-2"><input aria-label="Target agent" className="rounded border px-2" value={agent} onChange={event => setAgent(event.target.value)} /><Button disabled={!agent} onClick={async () => { await stageReleaseRollout(release.id, agent, release.release_hash); await load(); }}>Stage for next iteration</Button></div></section>)}
  </div>;
}
