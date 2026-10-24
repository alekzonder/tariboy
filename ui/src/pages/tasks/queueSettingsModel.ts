import { ApiError, type AgentPool } from "@/lib/api";
import { errorText } from "@/lib/errors";

/**
 * Pure readings behind the queue settings: what a refused bind means, and
 * which queue or request a form describes. The components render these;
 * nothing here touches the API.
 */

function detailList(error: ApiError, name: string): string[] {
  const list = error.details?.[name];
  return Array.isArray(list) ? list.map(String) : [];
}

/** Why a bind or clear was refused, as the Workflow tab acts on it. */
export type BindError =
  | { kind: "conflict" }
  | { kind: "pools"; names: string[]; message: string }
  | { kind: "secrets"; names: string[]; message: string }
  | { kind: "other"; message: string };

const BIND_ERROR_KIND: Record<string, "conflict" | "pools" | "secrets"> = {
  revision_conflict: "conflict",
  workflow_pool_empty: "pools",
  workflow_secret_missing: "secrets",
};

export function classifyBindError(err: unknown): BindError {
  const kind = err instanceof ApiError ? BIND_ERROR_KIND[err.code] : undefined;
  if (!(err instanceof ApiError) || !kind) return { kind: "other", message: errorText(err) };
  if (kind === "conflict") return { kind };
  return { kind, names: detailList(err, kind), message: err.message };
}

/** The named pools that still have no agents. */
export function stillEmptyPools(names: string[], pools: AgentPool[]): string[] {
  return names.filter((name) => !pools.find((pool) => pool.name === name)?.agents.length);
}
