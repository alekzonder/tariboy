import {
  ApiError,
  type AgentPool,
  type CreateQueueInput,
  type QueueWorkflow,
  type UpdateQueueInput,
  type WorkflowImageDefinition,
} from "@/lib/api";
import { errorText } from "@/lib/errors";
import { emptyPools, requiredPools } from "./workflowPools";

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

/** What the dialog loaded for a queue: its binding, the bound definition, its pools. */
export interface QueueState {
  binding: QueueWorkflow | null;
  definition: WorkflowImageDefinition | null;
  pools: AgentPool[];
}

/** A queue's load: its state, or why it failed. Absent while it loads. */
export type QueueEntry =
  | { state: QueueState; error?: undefined }
  | { state?: undefined; error: string };

export type QueueTab = "general" | "workflow" | "pools" | "environment" | "sources";

/** The queue list's second line: its workflow, `flexible`, or that it is unknown. */
export function queueSummary(entry: QueueEntry | undefined): string {
  if (!entry) return "…";
  if (!entry.state) return "unknown";
  const binding = entry.state.binding;
  return binding ? `${binding.name}@${binding.version}` : "flexible";
}

/** Whether a pool the queue's workflow uses has no agents. */
export function queueWarns(entry: QueueEntry | undefined): boolean {
  const state = entry?.state;
  return !!state?.definition && emptyPools(state.definition, state.pools).length > 0;
}

/**
 * The pools the Pools tab marks required: those the definition's statuses
 * own, and those a refused bind named.
 */
export function requiredWithMissing(
  definition: WorkflowImageDefinition | null | undefined,
  missing: string[],
): Map<string, string[]> {
  const required = new Map(definition ? requiredPools(definition) : []);
  for (const name of missing) if (!required.has(name)) required.set(name, []);
  return required;
}

/** The tabs of a queue: Sources only where a workflow is bound. */
export function queueTabs(binding: QueueWorkflow | null | undefined): QueueTab[] {
  const tabs: QueueTab[] = ["general", "workflow", "pools", "environment"];
  return binding ? [...tabs, "sources"] : tabs;
}

/** The tab to show: Sources falls back to General once the binding is gone. */
export function shownTab(tab: QueueTab, binding: QueueWorkflow | null | undefined): QueueTab {
  return tab === "sources" && !binding ? "general" : tab;
}

function splitList(value: FormDataEntryValue | null): string[] {
  return String(value ?? "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function field(values: FormData, name: string): string {
  return String(values.get(name) ?? "").trim();
}

/** The General tab's update, from its form. */
export function generalInput(values: FormData, revision: number): UpdateQueueInput {
  return {
    name: field(values, "name"),
    description: field(values, "description"),
    owners: splitList(values.get("owners")),
    responsible_agent: field(values, "responsible"),
    revision,
  };
}

/** A new queue, from its prefix and name and the rest of its form. */
export function newQueueInput(prefix: string, name: string, values: FormData): CreateQueueInput {
  return {
    prefix: prefix.trim(),
    name: name.trim(),
    owners: splitList(values.get("owners")),
    responsible_agent: field(values, "responsible"),
  };
}
