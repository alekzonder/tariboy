import { useApi } from "@/components/apiContext";
import { useKeyedRequest } from "./useKeyedRequest";

/**
 * Loads `GET /api/metrics/tasks/{key}` while `open`, and again on `retry()`.
 * There is no polling. The metrics of another task or host never show for
 * this one, also not while this one loads.
 */
export function useTaskMetrics(taskKey: string, open: boolean) {
  const api = useApi();
  const key = open && taskKey ? JSON.stringify([api.target?.id ?? "", taskKey]) : "";
  const { data, dataKey, ...state } = useKeyedRequest(key, (signal) =>
    api.metrics.getTaskMetrics(taskKey, signal),
  );
  const metrics = dataKey === key ? data : null;
  return {
    ...state,
    metrics,
    loading: !!key && !metrics && !state.error,
    updating: state.updating && !!metrics,
  };
}
