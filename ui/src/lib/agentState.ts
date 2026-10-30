import type { AgentRow, AgentStatus } from "./api";
import { letterTaskKey } from "./letters";
import { needsYouTask, type HostNeeds } from "./needsYou";
import type { StatusTone } from "./statusTone";

// Type-only API imports, like needsYou.ts: the sidebar reads this in the shell
// without pulling the API client into the entry chunk.

/** One thing an agent waits on the user for: a task in a customer status or
 *  paused, or an open question letter about a task. */
export interface NeedItem {
  key: string;
  at: string;
  /** The workflow status the task waits in, when it is a task. */
  status?: string;
  question: boolean;
}

export interface AgentNeeds {
  count: number;
  /** The oldest item, so the header can name it. */
  first?: NeedItem;
}

export type AgentState =
  | { kind: "error"; reason: string }
  | { kind: "needs"; needs: AgentNeeds }
  | { kind: "working"; since?: string }
  | { kind: "off" }
  | { kind: "idle" };

const openQuestion = (letter: HostNeeds["letters"][number]) =>
  letter.kind === "task.question" && !letter.answered_at && !letter.dead_at;

/** What this agent waits on the user for, on one host: its tasks that
 *  `needsYouTask` selects, plus its open questions about other tasks. One task
 *  counts once. A host that failed counts nothing. */
export function agentNeeds(host: HostNeeds | undefined, agentName: string): AgentNeeds {
  if (!host || host.error) return { count: 0 };
  const principal = `agent:${agentName}`;
  const items = new Map<string, NeedItem>();
  for (const task of host.tasks) {
    if (task.assignee !== principal || !needsYouTask(task)) continue;
    items.set(task.key, {
      key: task.key,
      at: task.updated_at,
      status: task.status,
      question: false,
    });
  }
  for (const letter of host.letters) {
    const key = letterTaskKey(letter);
    if (letter.from !== principal || !openQuestion(letter) || !key || items.has(key)) continue;
    items.set(key, { key, at: letter.created_at, question: true });
  }
  const sorted = [...items.values()].sort((left, right) => left.at.localeCompare(right.at));
  return sorted.length ? { count: sorted.length, first: sorted[0] } : { count: 0 };
}

type StateSource = Pick<
  AgentRow,
  "state" | "enabled" | "halt_kind" | "halt_reason" | "error_reason"
> & {
  budget?: { exhausted?: string[] | null };
};

function errorReason(source: StateSource): string {
  // An idle_limit halt is the loop stopping as configured, not a fault.
  if (source.halt_reason && source.halt_kind !== "idle_limit") return source.halt_reason;
  if (source.error_reason) return source.error_reason;
  const exhausted = source.budget?.exhausted ?? [];
  if (exhausted.length) return `Out of budget: ${exhausted.join(", ")}`;
  if (source.state === "error" || source.state === "failed") return `Agent state: ${source.state}`;
  return "";
}

/** The one state the sidebar row, the header and the Terminal banner show.
 *  The first that matches wins: error, needs you, working, off, idle. The
 *  workspace passes its fresher `status`; the sidebar has only the row. */
export function deriveAgentState({
  agent,
  status,
  needs,
}: {
  agent: AgentRow;
  status?:
    | (Pick<AgentStatus, "state" | "enabled" | "halt_kind" | "halt_reason" | "error_reason"> & {
        budget?: { exhausted?: string[] | null };
      })
    | null;
  needs: AgentNeeds;
}): AgentState {
  const source: StateSource = status ?? agent;
  const reason = errorReason(source);
  if (reason) return { kind: "error", reason };
  if (needs.count > 0) return { kind: "needs", needs };
  if (source.state === "running") {
    return agent.current_iteration_started_at
      ? { kind: "working", since: agent.current_iteration_started_at }
      : { kind: "working" };
  }
  if (source.enabled === false) return { kind: "off" };
  return { kind: "idle" };
}

const STATE_TONE: Record<AgentState["kind"], StatusTone> = {
  error: "danger",
  needs: "attention",
  working: "live",
  off: "quiet",
  idle: "quiet",
};

/** The dot and pill color of a derived state. */
export const agentStateTone = (state: AgentState): StatusTone => STATE_TONE[state.kind];
