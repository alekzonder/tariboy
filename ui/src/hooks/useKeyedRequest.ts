import { useEffect, useEffectEvent, useState } from "react";

interface Answer<T> {
  /** The request this answers: the key and the attempt. */
  request: string;
  key: string;
  data: T | null;
  error: unknown;
  updatedAt: Date | null;
}

const NO_ANSWER: Answer<never> = {
  request: "",
  key: "",
  data: null,
  error: null,
  updatedAt: null,
};

/**
 * Runs `load` once for each `key`, and again on `retry()`. An empty key makes
 * no request. A new request cancels the older one, so an older answer never
 * replaces the current one. While a new request runs, the last data stays,
 * with `updating` set; `dataKey` names the key that data answered.
 */
export function useKeyedRequest<T>(key: string, load: (signal: AbortSignal) => Promise<T>) {
  const [attempt, setAttempt] = useState(0);
  const request = key ? JSON.stringify([key, attempt]) : "";
  const [answer, setAnswer] = useState<Answer<T>>(NO_ANSWER);
  const run = useEffectEvent(load);

  useEffect(() => {
    if (!request) return;
    const controller = new AbortController();
    const settle = (next: (prev: Answer<T>) => Answer<T>) => {
      if (!controller.signal.aborted) setAnswer(next);
    };
    run(controller.signal).then(
      (data) => settle(() => ({ request, key, data, error: null, updatedAt: new Date() })),
      (error: unknown) => settle((prev) => ({ ...prev, request, error })),
    );
    return () => controller.abort();
  }, [request, key]);

  const answered = answer.request === request;
  const pending = !!request && !answered;
  // Without a key the last data belongs to a request nobody makes any more.
  const { data, updatedAt, key: dataKey } = request ? answer : NO_ANSWER;
  return {
    data,
    dataKey,
    error: answered ? answer.error : null,
    loading: pending && !data,
    updating: pending && !!data,
    updatedAt,
    retry: () => setAttempt((n) => n + 1),
  };
}
