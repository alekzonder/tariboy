import { useMemo, type ReactNode } from "react";
import { createApi, type ApiClient } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { ApiContext } from "./apiContext";

type ApiProviderProps =
  | { target: Daemon | null; client?: never; children: ReactNode }
  | { client: ApiClient; target?: never; children: ReactNode };

// ApiProvider fixes the server for everything below it. Server-selection
// boundaries (the active host, a route host, an explicit agent host, a dialog
// with its own target host) mount one; descendants call useApi() and never pass
// a target themselves. An equal target keeps the same client, so ordinary
// re-renders do not restart requests or subscriptions; a new endpoint or token
// for the same host gives a new client without remounting the tree.
export function ApiProvider(props: ApiProviderProps) {
  const target = props.client ? null : props.target;
  const identity = target ? [target.id, target.baseURL, target.token].join("\n") : "";
  // The client depends on the target's identity and credentials only.
  const override = props.client;
  // oxlint-disable-next-line react/exhaustive-deps -- keyed by identity on purpose: an equal target keeps the same client
  const created = useMemo(() => (override ? null : createApi(target)), [override, identity]);
  return <ApiContext.Provider value={props.client ?? created}>{props.children}</ApiContext.Provider>;
}
