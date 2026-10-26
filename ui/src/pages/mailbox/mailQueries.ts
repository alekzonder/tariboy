import { useCallback } from "react";
import { infiniteQueryOptions, queryOptions, useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useMailSocket } from "@/hooks/useMailSocket";
import type { ApiClient, MailBox } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { apiServerKey } from "@/lib/queryClient";

// `as` is "" for the customer: the daemon's default principal.
export const mailKeys = {
  server: (target: Daemon | null) => [...apiServerKey(target), "mail"] as const,
  box: (target: Daemon | null, as: string, box: MailBox) =>
    [...mailKeys.server(target), as, "box", box] as const,
  unread: (target: Daemon | null, as: string) =>
    [...mailKeys.server(target), as, "unread"] as const,
};

export const mailQueries = {
  // Sent and Archive page back by created_at; the Inbox is one page.
  box: (api: ApiClient, as: string, box: MailBox) =>
    infiniteQueryOptions({
      queryKey: mailKeys.box(api.target, as, box),
      queryFn: ({ signal, pageParam }) =>
        api.mail.list({ box, as: as || undefined, before: pageParam || undefined }, signal),
      initialPageParam: "",
      getNextPageParam: (last) => last.next_before || undefined,
      enabled: api.ready,
    }),
  unread: (api: ApiClient, as: string) =>
    queryOptions({
      queryKey: mailKeys.unread(api.target, as),
      queryFn: ({ signal }) => api.mail.unread(as || undefined, signal),
    }),
};

/** Refetches this host's mail on every letter to or from `as` (every letter for
 *  the customer, whose principal the UI does not know) and on every reconnect.
 *  Returns the refetch, for after an action. */
export function useLiveMail(as: string): () => Promise<void> {
  const api = useApi();
  const client = useQueryClient();
  const target = api.target;
  const refresh = useCallback(
    () => client.invalidateQueries({ queryKey: mailKeys.server(target) }),
    [client, target],
  );
  useMailSocket({
    enabled: api.ready,
    onHint: (hint) => {
      if (!as || hint.from === as || hint.to === as) void refresh();
    },
    onOpen: () => void refresh(),
  });
  return refresh;
}
