import { useCallback, useEffect, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Link } from "react-router-dom";
import { Select,SelectContent,SelectItem,SelectTrigger,SelectValue } from "@/components/ui/select";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { RunStateActions } from "@/components/AgentControls";
import AgentSettings from "@/pages/AgentSettings";
import { agentGetOn, agentImageCancelOn,agentImageSetOn,agentImageStatusGetOn,agentPost,agentPostOn,listImagesOn,setAgentCwdOn,type AgentImageStatus,type ImageRow } from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import { useAgentName, useAgentStatus } from "@/lib/agent";
import type { Daemon } from "@/lib/daemons";
import type { AgentBudgetStatus, AgentView } from "@/lib/types";
import { serverPath } from "@/lib/terminalsHost";

// A digest is a 64-hex-character hash: shown whole it dominates the identity
// row and wraps across lines. Keep enough of both ends to compare against a
// registry listing by eye; the untruncated value stays in the a11y tree.
function shortDigest(digest: string): string {
  return digest.length > 24 ? `${digest.slice(0, 14)}…${digest.slice(-8)}` : digest;
}

export default function AgentConfigurationTab({
  target,
  refresh,
}: {
  target: Daemon | null;
  refresh: () => void;
}) {
  const name = useAgentName();
  const { status } = useAgentStatus();
  const [agent, setAgent] = useState<AgentView | null>(null);
  const [cwd, setCwd] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
	const [budget, setBudget] = useState<AgentBudgetStatus | null>(null);
	const [budgetSaving, setBudgetSaving] = useState(false);
	const [budgetError, setBudgetError] = useState("");
  const [images,setImages]=useState<ImageRow[]>([]);const [imageStatus,setImageStatus]=useState<AgentImageStatus|null>(null);const [selectedImage,setSelectedImage]=useState("");const [imageSaving,setImageSaving]=useState(false);
  const targetId = target?.id;
  const targetLabel = target?.label;
  const targetBaseURL = target?.baseURL;
  const targetToken = target?.token;
  // Host registry refreshes replace the descriptor object even when its API
  // endpoint is unchanged. Keep a stable request target so those parent
  // renders cannot reload the saved CWD over an in-progress draft.
  const requestTarget = useMemo<Daemon | null>(
    () => targetId === undefined ? null : {
      id: targetId,
      label: targetLabel ?? targetId,
      baseURL: targetBaseURL ?? "",
      token: targetToken ?? "",
    },
    [targetBaseURL, targetId, targetLabel, targetToken],
  );

  const load = useCallback(async () => {
    try {
      const next = await agentGetOn<AgentView>(requestTarget, name, "");
      setAgent(next);
		setBudget(next.budget ?? null);
      setCwd(next.cwd);
    } catch {
      setAgent(null);
    }
  }, [name, requestTarget]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const loadImages = useCallback(async () => {
    try {
      const [listing, state] = await Promise.all([
        listImagesOn(requestTarget), agentImageStatusGetOn(requestTarget, name),
      ]);
      setImages(listing.images ?? []);
      setImageStatus(state);
      setSelectedImage(state.pending.ref || state.current.ref);
    } catch {
      setImageStatus(null);
    }
  }, [name, requestTarget]);
  useEffect(() => { void Promise.resolve().then(loadImages); }, [loadImages]);
  const imageHref = (ref: string) => {
    const split = ref.lastIndexOf(":");
    const base = serverPath(targetId ?? "", "images");
    return split < 0 ? base : `${base}/${encodeURIComponent(ref.slice(0, split))}/${encodeURIComponent(ref.slice(split + 1))}`;
  };
  const scheduleImage = async () => {
    if (!selectedImage) return;
    setImageSaving(true);
    await guard("image assignment", async () => {
      await agentImageSetOn(requestTarget, name, selectedImage);
      await loadImages();
      refresh();
    });
    setImageSaving(false);
  };
  const cancelImage = async () => {
    setImageSaving(true);
    await guard("pending image cancellation", async () => {
      await agentImageCancelOn(requestTarget, name);
      await loadImages();
      refresh();
    });
    setImageSaving(false);
  };

  const stopped = status
    ? status.state === "stopped"
    : agent?.enabled === false ||
      (agent?.enabled === undefined && agent?.state === "stopped");

  const saveCwd = async () => {
    setSaving(true);
    setError("");
    try {
      await setAgentCwdOn(requestTarget, name, cwd.trim());
      await load();
      refresh();
    } catch (cause) {
      // The section copy leads so the outcome is unambiguous, but the server's
      // reason follows it: only the daemon knows WHY this path was rejected.
      const reason = cause instanceof Error ? cause.message : String(cause);
      setError(`Working directory was not saved. Fix the path and try again.${reason ? ` ${reason}` : ""}`);
    } finally {
      setSaving(false);
    }
  };

  const copyDigest = async (digest: string) => {
    try {
      await navigator.clipboard.writeText(digest);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

	const saveBudget = async () => {
		if (!budget) return;
		setBudgetSaving(true);
		setBudgetError("");
		try {
			const next = await agentPostOn<AgentBudgetStatus>(requestTarget, name, "budget", {
				hour_usd: String(budget.hour_usd), day_usd: String(budget.day_usd),
				week_usd: String(budget.week_usd), month_usd: String(budget.month_usd),
			});
			setBudget(next); await load(); refresh();
		} catch (cause) {
			const reason = cause instanceof Error ? cause.message : String(cause);
			setBudgetError(`Agent budgets were not saved.${reason ? ` ${reason}` : ""}`);
		} finally { setBudgetSaving(false); }
	};
	const budgetRows = budget ? ([
		["Hour", "hour", budget.hour_spent_usd, budget.hour_usd], ["Day", "day", budget.day_spent_usd, budget.day_usd],
		["Week", "week", budget.week_spent_usd, budget.week_usd], ["Month", "month", budget.month_spent_usd, budget.month_usd],
	] as const) : [];
	const exhaustedPeriods = budget?.exhausted ?? [];

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <h2 className="font-semibold">Configuration</h2>
      <section className="rounded-lg border p-4">
        <h3 className="text-base font-semibold">Run state</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          The master switch permits the agent to run; Loop schedules new autonomous iterations.
        </p>
        {agent && (
          <div className="mt-4 grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <p className="text-sm">
                <span className="text-muted-foreground">Master switch</span>{" "}
                <span data-testid="master-switch-state" className="font-medium">
                  {agent.enabled === false ? "Disabled" : "Enabled"}
                </span>
              </p>
              <RunStateActions
                name={name}
                refresh={() => {
                  void load();
                  refresh();
                }}
              />
              <p className="text-xs text-muted-foreground">Takes effect immediately.</p>
            </div>
            <div className="space-y-1.5">
              <p className="text-sm">
                <span className="text-muted-foreground">Loop</span>{" "}
                <span data-testid="loop-state" className="font-medium">
                  {agent.loop_enabled ? "Enabled" : "Disabled"}
                </span>
              </p>
              <div>
                {agent.loop_enabled ? (
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => void guard("disable", () => agentPost(name, "loop/disable")).then(load)}
                  >
                    Disable
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    onClick={() => void guard("enable", () => agentPost(name, "loop/enable")).then(load)}
                  >
                    Enable
                  </Button>
                )}
              </div>
              <p className="text-xs text-muted-foreground">Takes effect immediately.</p>
            </div>
          </div>
        )}
      </section>
		{budget && <section className="rounded-lg border p-4">
			<h3 className="text-base font-semibold">Agent budgets (USD)</h3>
			<p className="mt-1 text-sm text-muted-foreground">Zero means Unlimited. Each configured calendar limit applies independently.</p>
			<div className="mt-4 space-y-2">
				{budgetRows.map(([label, key, spent, limit]) => <label key={key} className="flex items-center gap-2 text-sm"><span className="w-14">{label}</span><span>{spent.toFixed(2)} /</span><input aria-label={`${label} budget`} className="w-28 rounded border px-2 py-1" type="number" min="0" step="0.01" value={limit} onChange={(event) => setBudget({ ...budget, [`${key}_usd`]: Number(event.target.value) })} /><span className="text-muted-foreground">{limit === 0 ? "Unlimited" : "USD"}</span></label>)}
			</div>
			{exhaustedPeriods.length > 0 && <p role="status" className="mt-3 text-sm text-destructive">Out of budget: {exhaustedPeriods.join(", ")}</p>}
			{budgetError && <p role="alert" className="mt-3 text-sm text-destructive">{budgetError}</p>}
			<Button className="mt-4" disabled={budgetSaving} onClick={() => void saveBudget()}>{budgetSaving ? "Saving…" : "Save agent budgets"}</Button>
		</section>}
      <section className="rounded-lg border p-4">
        <h3 className="text-base font-semibold">Identity and location</h3>
        {agent && (
          <dl className="mt-4 grid gap-4 text-sm sm:grid-cols-2">
            <div><dt className="text-muted-foreground">Image</dt><dd className="break-all">{agent.image}</dd></div>
            <div>
              <dt className="text-muted-foreground">Image digest</dt>
              <dd className="mt-1 flex flex-wrap items-center gap-2">
                {agent.digest ? (
                  <>
                    <span
                      data-testid="image-digest"
                      aria-hidden="true"
                      title={agent.digest}
                      className="font-mono text-xs break-all"
                    >
                      {shortDigest(agent.digest)}
                    </span>
                    <span className="sr-only">{agent.digest}</span>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => void copyDigest(agent.digest)}
                    >
                      Copy digest
                    </Button>
                    {copied && (
                      <span role="status" className="text-xs text-muted-foreground">Digest copied</span>
                    )}
                  </>
                ) : (
                  <span>—</span>
                )}
              </dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-muted-foreground">Working directory</dt>
              <dd className="mt-1">
                {stopped ? (
                  <div className="space-y-1.5">
                    <div className="flex items-start gap-2">
                      <PathAutocomplete
                        value={cwd}
                        onChange={(value) => {
                          setCwd(value);
                          setError("");
                        }}
                        daemon={requestTarget}
                        aria-label="Working directory"
                        className="min-w-0 flex-1"
                      />
                      <Button
                        type="button"
                        className="h-9"
                        disabled={saving}
                        onClick={() => void saveCwd()}
                      >
                        {saving ? "Saving…" : "Save working directory"}
                      </Button>
                    </div>
                    {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
                    <p className="text-xs text-muted-foreground">
                      Takes effect on the next start. Existing files stay where they are.
                    </p>
                  </div>
                ) : (
                  <div className="space-y-1">
                    <p className="break-all">{agent.cwd || "—"}</p>
                    <p className="text-xs text-muted-foreground">
                      Stop the agent before changing its working directory.
                    </p>
                  </div>
                )}
              </dd>
            </div>
          </dl>
        )}
      </section>
      <section className="rounded-lg border p-4">
        <h3 className="text-base font-semibold">Agent image</h3>
        <p className="mt-1 text-sm text-muted-foreground">Select an already-built image. It becomes active before the next iteration and does not change runtime settings.</p>
        {imageStatus&&<div className="mt-4 space-y-3 text-sm">
          <div>Current: <Link className="font-mono text-primary hover:underline" to={imageHref(imageStatus.current.ref)}>{imageStatus.current.ref}</Link></div>
          {imageStatus.pending.ref&&<div>Pending: <Link className="font-mono text-primary hover:underline" to={imageHref(imageStatus.pending.ref)}>{imageStatus.pending.ref}</Link>{imageStatus.pending.error&&<p role="alert" className="mt-1 text-destructive">{imageStatus.pending.error}</p>}</div>}
          <div className="flex flex-wrap gap-2"><Select value={selectedImage} onValueChange={setSelectedImage}><SelectTrigger aria-label="Agent image" className="w-72"><SelectValue placeholder="Select image"/></SelectTrigger><SelectContent>{images.map(item=>{const ref=`${item.name}:${item.tag}`;return <SelectItem key={ref} value={ref}>{ref}</SelectItem>})}</SelectContent></Select><Button disabled={imageSaving||!selectedImage} onClick={()=>void scheduleImage()}>{imageStatus.pending.error?"Retry":"Use next iteration"}</Button>{imageStatus.pending.ref&&<Button variant="outline" disabled={imageSaving} onClick={()=>void cancelImage()}>Cancel pending</Button>}</div>
        </div>}
      </section>
      <AgentSettings />
    </div>
  );
}
