import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { getIterationJudgeReviewsOn, getJudgeAutomation, reviewIterationOn } from "@/lib/judge";
import { agentGetOn } from "@/lib/api";
import type { AgentStatus, IterationJudgeProjection, IterationJudgeReview } from "@/lib/types";
import { hostToParam, paramToHost, targetFor } from "@/lib/terminalsHost";
import { fmtDateTime } from "@/lib/time";

function reviewPath(hostId: string, review: IterationJudgeReview): string {
  const query = new URLSearchParams({ target: review.target_id });
  return `/servers/${encodeURIComponent(hostToParam(hostId))}/settings/advanced/judges/${encodeURIComponent(review.run_id)}?${query}`;
}

export function IterationJudgePanel({ agentName, iterationId, terminal, judge }: {
  agentName: string;
  iterationId: string;
  terminal: boolean;
  judge: IterationJudgeProjection;
}) {
  const { hostId: hostParam = "local" } = useParams();
  const hostId = paramToHost(hostParam);
  const descriptor = targetFor(hostId);
  const requestKey = `${hostId}\0${descriptor?.baseURL ?? ""}\0${descriptor?.token ?? ""}\0${agentName}\0${iterationId}`;
  const [reviewState, setReviewState] = useState<{ key: string; reviews: IterationJudgeReview[]; loaded: boolean }>({ key: requestKey, reviews: [], loaded: false });
  const [acceptedState, setAcceptedState] = useState<{ key: string; runId: string }>({ key: requestKey, runId: "" });
  const [errorState, setErrorState] = useState<{ key: string; value: string }>({ key: requestKey, value: "" });
  const [postingKey, setPostingKey] = useState("");
  const [workerReason, setWorkerReason] = useState<{ key: string; value: string }>({ key: requestKey, value: "" });
  const posting = useRef("");
  const accepted = useRef({ key: requestKey, runId: "" });
  const historyGeneration = useRef(0);
  const currentKey = useRef(requestKey);
  const reviews = reviewState.key === requestKey ? reviewState.reviews : [];
  const historyLoaded = reviewState.key === requestKey && reviewState.loaded;
  const queued = acceptedState.key === requestKey && acceptedState.runId !== "";
  const error = errorState.key === requestKey ? errorState.value : "";
  const isPosting = postingKey === requestKey;

  const load = async (key = requestKey) => {
    const generation = ++historyGeneration.current;
    const requestTarget = targetFor(hostId);
    const result = await getIterationJudgeReviewsOn(requestTarget, agentName, iterationId);
    const reviews = result.reviews ?? [];
    if (currentKey.current === key && historyGeneration.current === generation) {
      setReviewState({ key, reviews, loaded: true });
      const pending = accepted.current;
      if (pending.key === key && pending.runId && reviews.some((review) => review.run_id === pending.runId)) {
        accepted.current = { key, runId: "" };
        setAcceptedState({ key, runId: "" });
      }
    }
    return reviews;
  };

  useEffect(() => {
    let current = true;
    currentKey.current = requestKey;
    const requestTarget = targetFor(hostId);
    const refresh = () => {
      const generation = ++historyGeneration.current;
      void getIterationJudgeReviewsOn(requestTarget, agentName, iterationId)
      .then((result) => {
        if (!current || historyGeneration.current !== generation) return;
        const reviews = result.reviews ?? [];
        setReviewState({ key: requestKey, reviews, loaded: true });
        const pending = accepted.current;
        if (pending.key === requestKey && pending.runId && reviews.some((review) => review.run_id === pending.runId)) {
          accepted.current = { key: requestKey, runId: "" };
          setAcceptedState({ key: requestKey, runId: "" });
        }
      })
      .catch(() => { /* list projection remains usable */ });
    };
    refresh();
    const timer = window.setInterval(refresh, 1500);
    return () => { current = false; window.clearInterval(timer); };
  }, [agentName, hostId, iterationId, requestKey]);

  const start = async () => {
    if (posting.current === requestKey) return;
    posting.current = requestKey;
    const key = requestKey;
    setPostingKey(key);
    setErrorState({ key, value: "" });
    let started: Awaited<ReturnType<typeof reviewIterationOn>>;
    try {
      started = await reviewIterationOn(targetFor(hostId), iterationId);
    } catch (cause) {
      if (currentKey.current === key) setErrorState({ key, value: cause instanceof Error ? cause.message : String(cause) });
      return;
    } finally {
      if (posting.current === key) posting.current = "";
      if (currentKey.current === key) setPostingKey("");
    }
    if (currentKey.current !== key) return;
    accepted.current = { key, runId: started.id };
    setAcceptedState({ key, runId: started.id });
    try {
      await load(key);
    } catch { /* accepted POST remains queued; periodic history refresh will reconcile it */ }
  };

  const latest = judge.latest_completed;
  const historyActive = reviews.find((review) => review.state === "pending" || review.state === "running" || review.pending > 0);
  const active = historyLoaded ? historyActive : judge.active;

  useEffect(() => {
    if (!active?.pending) return;
    let current = true;
    const target = targetFor(hostId);
    void getJudgeAutomation(target).then(async (automation) => {
      const raw = automation.revision?.canonical_json;
      if (!raw) return "Waiting for configured Judge workers; worker status is unknown.";
      const workers = (JSON.parse(raw) as { judge?: { workers?: string[] } }).judge?.workers ?? [];
      for (const worker of workers) {
        try {
          const status = await agentGetOn<AgentStatus>(target, worker, "status");
          if (!status.loop_enabled) return `Waiting: Judge worker ${worker} has Autopilot disabled.`;
        } catch {
          return `Waiting: Judge worker ${worker} status is unknown.`;
        }
      }
      return "Waiting for configured Judge workers.";
    }).then((value) => { if (current && value) setWorkerReason({ key: requestKey, value }); })
      .catch(() => { if (current) setWorkerReason({ key: requestKey, value: "Waiting for configured Judge workers; worker status is unknown." }); });
    return () => { current = false; };
  }, [active?.pending, hostId, requestKey]);

  return (
    <section aria-label="Judge review" className="space-y-2 rounded border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-medium">Judge review</h2>
        {latest && <><Badge variant="outline">Score {latest.score ?? "—"}</Badge><span>{latest.verdict || "—"}</span><time dateTime={latest.created_at}>{fmtDateTime(latest.created_at)}</time></>}
      </div>
      {active ? (
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary">{active.completed > 0 ? `${active.completed} responses` : "Queued"}</Badge>
          {active.pending > 0 && <span className="text-muted-foreground">{workerReason.key === requestKey && workerReason.value || "Checking configured Judge workers…"}</span>}
          <Link className="underline" to={reviewPath(hostId, active)}>Open active review</Link>
        </div>
      ) : queued ? (
        <p>Queued — waiting for the review target to be confirmed.</p>
      ) : terminal ? (
        <Button size="sm" disabled={isPosting} onClick={() => void start()}>{isPosting ? "Queuing Judge review…" : error ? "Retry Judge review" : "Run Judge review"}</Button>
      ) : (
        <p className="text-muted-foreground">Judge review is available after this iteration finishes.</p>
      )}
      {error && <p role="alert" className="text-destructive">Could not start Judge review: {error}</p>}
      {reviews.length > 0 && (
        <div>
          <h3 className="text-xs font-medium text-muted-foreground">Review history</h3>
          <ul>{reviews.map((review) => <li key={`${review.run_id}:${review.target_id}`}><Link className="underline" to={reviewPath(hostId, review)}>{fmtDateTime(review.created_at)} · {review.verdict || review.state} · {review.score ?? "—"}</Link></li>)}</ul>
        </div>
      )}
    </section>
  );
}
