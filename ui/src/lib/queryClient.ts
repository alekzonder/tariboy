import { QueryCache, QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api";
import type { Daemon } from "./daemons";
import { subscribeResume } from "./resume";

export const apiServerKey = (target: Daemon | null) => ["server", target?.id ?? "local"] as const;
export const isAccessError = (error: unknown) =>
  error instanceof ApiError && (error.status === 401 || error.status === 403);

export function createQueryClient() {
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 0,
        gcTime: 5 * 60_000,
        retry: false,
        refetchOnWindowFocus: false,
        refetchOnReconnect: false,
        networkMode: "always",
      },
      mutations: { retry: false, networkMode: "always" },
    },
    queryCache: new QueryCache({
      onError(error, failed) {
        if (!isAccessError(error)) return;
        // A refused read must not leave a previously authorized snapshot visible.
        const queryKey = failed.queryKey.slice(0, 2);
        void client.cancelQueries({ queryKey });
        for (const query of client.getQueryCache().findAll({ queryKey })) {
          query.setState({ data: undefined, error, status: "error", fetchStatus: "idle" });
        }
      },
    }),
  });
  return client;
}

/** Clear observer-held snapshots before removing the cache entries. */
export function clearServerQueries(client: QueryClient, server: string) {
  const queryKey = ["server", server];
  for (const query of client.getQueryCache().findAll({ queryKey })) query.reset();
  client.removeQueries({ queryKey });
}

export const queryClient = createQueryClient();
// Coming back to the app rereads what is on screen: a read made over a dead
// connection must not leave stale data until the next hint.
subscribeResume(() => void queryClient.invalidateQueries());
