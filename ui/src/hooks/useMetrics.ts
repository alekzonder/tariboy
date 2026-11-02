import { useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import type { MetricsFilter, MetricsReport } from "@/lib/api";

interface Answer {
  /** The request this answers: host, period, filters and attempt. */
  request: string;
  report: MetricsReport | null;
  error: unknown;
  updatedAt: Date | null;
}

const NO_ANSWER: Answer = { request: "", report: null, error: null, updatedAt: null };

/**
 * Loads `GET /api/metrics` once for each host, period and filter, and again on
 * `retry()`. It does not poll: the request is heavy. A new request cancels the
 * older one, so an older answer never replaces the current one. While a new
 * request runs, the last report stays, with `updating` set. `filter` null
 * (an unusable custom period) makes no request.
 */
export function useMetrics(filter: MetricsFilter | null) {
  const api = useApi();
  const [attempt, setAttempt] = useState(0);
  const { since = "", until = "", agent = "", image = "", workflow = "" } = filter ?? {};
  const request = filter
    ? JSON.stringify([api.target?.id ?? "", since, until, agent, image, workflow, attempt])
    : "";
  const [answer, setAnswer] = useState<Answer>(NO_ANSWER);

  useEffect(() => {
    if (!request) return;
    const controller = new AbortController();
    api.metrics.getMetrics({ since, until, agent, image, workflow }, controller.signal).then(
      (report) => {
        if (!controller.signal.aborted) {
          setAnswer({ request, report, error: null, updatedAt: new Date() });
        }
      },
      (error: unknown) => {
        if (!controller.signal.aborted) setAnswer((prev) => ({ ...prev, request, error }));
      },
    );
    return () => controller.abort();
  }, [api, request, since, until, agent, image, workflow]);

  const answered = answer.request === request;
  const pending = !!request && !answered;
  // Without a usable period the last report belongs to another period.
  const { report, updatedAt } = request ? answer : NO_ANSWER;
  return {
    report,
    error: answered ? answer.error : null,
    loading: pending && !report,
    updating: pending && !!report,
    updatedAt,
    retry: () => setAttempt((n) => n + 1),
  };
}
