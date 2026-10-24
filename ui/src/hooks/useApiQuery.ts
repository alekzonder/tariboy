import { useEffect } from "react";
import {
  useQuery,
  useQueryClient,
  type QueryClient,
  type QueryKey,
  type UseQueryOptions,
} from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { getLocalBaseURL } from "@/lib/api";
import { apiServerKey } from "@/lib/queryClient";

type Connection = { endpoint: string; token: string; ready: boolean };
const connections = new WeakMap<QueryClient, Map<string, Connection>>();

/** Runs a read against the server of the nearest ApiProvider. */
export function useApiQuery<T, K extends QueryKey>(options: UseQueryOptions<T, Error, T, K>) {
  const client = useQueryClient();
  const api = useApi();
  const target = api.target;
  const ready = api.ready;
  const server = target?.id ?? "local";
  const endpoint = target?.baseURL ?? getLocalBaseURL();
  const token = target?.token ?? "";
  const previous = connections.get(client)?.get(server);
  const changedAccount = ready && previous !== undefined && previous.token !== token;
  const result = useQuery<T, Error, T, K>({
    ...options,
    // Clear the observer's memoized placeholder too, not only its query cache.
    placeholderData: changedAccount ? undefined : options.placeholderData,
    enabled: ready && options.enabled !== false,
  });

  useEffect(() => {
    let hosts = connections.get(client);
    if (!hosts) {
      hosts = new Map();
      connections.set(client, hosts);
    }
    const old = hosts.get(server);
    hosts.set(server, { endpoint, token: ready ? token : (old?.token ?? token), ready });
    if (!old || (old.endpoint === endpoint && (!ready || old.token === token))) return;
    const queryKey = apiServerKey(target);
    if (old.ready) void client.cancelQueries({ queryKey });
    if (ready && old.token !== token) {
      for (const query of client.getQueryCache().findAll({ queryKey })) query.reset();
    }
    // All observers first receive the new explicit endpoint, then reads resume.
    if (ready && (old.ready || old.token !== token))
      queueMicrotask(() => {
        void client.invalidateQueries({ queryKey }, { cancelRefetch: false });
      });
  }, [client, endpoint, ready, server, target, token]);

  return changedAccount ? { ...result, data: undefined } : result;
}
