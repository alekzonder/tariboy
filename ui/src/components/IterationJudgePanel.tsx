import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { getIterationJudgeReviewsOn, reviewIterationOn } from "@/lib/judge";
import type { IterationJudgeProjection, IterationJudgeReview } from "@/lib/types";
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
  const target = useMemo(() => targetFor(hostId), [hostId]);
  const requestKey = `${hostId}\0${agentName}\0${iterationId}`;
  const [reviewState, setReviewState] = useState<{ key: string; reviews: IterationJudgeReview[] }>({ key: requestKey, reviews: [] });
  const [queuedState, setQueuedState] = useState<{ key: string; value: boolean }>({ key: requestKey, value: false });
  const [errorState, setErrorState] = useState<{ key: string; value: string }>({ key: requestKey, value: "" });
  const posting = useRef(false);
  const currentKey = useRef(requestKey);
  const reviews = reviewState.key === requestKey ? reviewState.reviews : [];
  const queued = queuedState.key === requestKey && queuedState.value;
  const error = errorState.key === requestKey ? errorState.value : "";

  const load = async (key = requestKey) => {
    const result = await getIterationJudgeReviewsOn(target, agentName, iterationId);
    if (currentKey.current === key) setReviewState({ key, reviews: result.reviews ?? [] });
    return result.reviews ?? [];
  };

  useEffect(() => {
    let current = true;
    currentKey.current = requestKey;
    posting.current = false;
    void getIterationJudgeReviewsOn(target, agentName, iterationId)
      .then((result) => { if (current) setReviewState({ key: requestKey, reviews: result.reviews ?? [] }); })
      .catch(() => { /* list projection remains usable */ });
    return () => { current = false; };
  }, [agentName, iterationId, requestKey, target]);

  const start = async () => {
    if (posting.current) return;
    posting.current = true;
    const key = requestKey;
    setErrorState({ key, value: "" });
    try {
      const started = await reviewIterationOn(target, iterationId);
      if (currentKey.current === key) setQueuedState({ key, value: true });
      await load(key).then((next) => {
        if (currentKey.current === key && next.some((review) => review.run_id === started.id)) setQueuedState({ key, value: false });
      });
    } catch (cause) {
      if (currentKey.current === key) setErrorState({ key, value: cause instanceof Error ? cause.message : String(cause) });
    } finally {
      posting.current = false;
    }
  };

  const latest = judge.latest_completed;
  const active = judge.active
    ?? reviews.find((review) => review.state === "pending" || review.state === "running" || review.pending > 0);

  return (
    <section aria-label="Judge review" className="space-y-2 rounded border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-medium">Judge review</h2>
        {latest && <><Badge variant="outline">Score {latest.score ?? "—"}</Badge><span>{latest.verdict || "—"}</span><time dateTime={latest.created_at}>{fmtDateTime(latest.created_at)}</time></>}
      </div>
      {active ? (
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary">{active.completed > 0 ? `${active.completed} responses` : "Queued"}</Badge>
          {active.pending > 0 && <span className="text-muted-foreground">Waiting for configured Judge workers; disabled workers stay disabled.</span>}
          <Link className="underline" to={reviewPath(hostId, active)}>Open active review</Link>
        </div>
      ) : queued ? (
        <p>Queued — waiting for the review target to be confirmed.</p>
      ) : terminal ? (
        <Button size="sm" onClick={() => void start()}>{error ? "Retry Judge review" : "Run Judge review"}</Button>
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
