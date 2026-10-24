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
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const created = useMemo(() => (props.client ? null : createApi(target)), [props.client, identity]);
  return <ApiContext.Provider value={props.client ?? created}>{props.children}</ApiContext.Provider>;
}
