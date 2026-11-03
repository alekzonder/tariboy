import { useApi } from "@/components/apiContext";
import type { MetricsFilter } from "@/lib/api";
import { useKeyedRequest } from "./useKeyedRequest";

/**
 * Loads `GET /api/metrics` once for each host, period and filter, and again on
 * `retry()`. It does not poll: the request is heavy. A new request cancels the
 * older one, so an older answer never replaces the current one. While a new
 * request runs, the last report stays, with `updating` set. `filter` null
 * (an unusable custom period) makes no request.
 */
export function useMetrics(filter: MetricsFilter | null) {
  const api = useApi();
  const { since = "", until = "", agent = "", image = "", workflow = "" } = filter ?? {};
  const key = filter
    ? JSON.stringify([api.target?.id ?? "", since, until, agent, image, workflow])
    : "";
  const state = useKeyedRequest(key, (signal) =>
    api.metrics.getMetrics({ since, until, agent, image, workflow }, signal),
  );
  return { ...state, report: state.data };
}
