/**
 * The status rule of the Tariboy style layer, in one place.
 *
 * A fill is reserved for two things: what is ALIVE (an agent running, a task in
 * progress) and what NEEDS A PERSON (a task waiting on the customer, a failed
 * agent, an exhausted budget). Everything else — queued, stopped, open, done —
 * is quiet text on no background, and cancelled is quieter still.
 *
 * Every status surface in the app (sidebar rows, the agent header, task rows,
 * task tables) reads its colors from here, so the rule is changed in one edit
 * rather than re-decided per row. The tone is what callers reason about; the
 * class tables below are the only place a token name is spelled out.
 */
export type StatusTone =
  /** running / in progress — alive, gets the green fill */
  | "live"
  /** waiting on the customer — needs a person, gets the accent fill */
  | "attention"
  /** failed / out of budget — needs a person, gets the red fill */
  | "danger"
  /** queued, stopped, open, done — quiet text */
  | "quiet"
  /** cancelled — quiet text, dimmed further */
  | "faint";

/** Agent lifecycle state → tone. An exhausted budget outranks the state: the
 *  agent may still say "running", but the thing to act on is the budget. */
export function agentTone(state: string, outOfBudget = false): StatusTone {
  if (outOfBudget) return "danger";
  if (state === "running") return "live";
  if (state === "failed" || state === "error") return "danger";
  return "quiet";
}

/** Task status → tone. */
export function taskTone(status: string): StatusTone {
  if (status === "in_progress") return "live";
  if (status === "wait_customer") return "attention";
  if (status === "cancelled") return "faint";
  return "quiet";
}

/** Task priority → tone. P0/P1 is the only priority that earns a fill. */
export function priorityTone(priority: string): StatusTone {
  const rank = priority.trim().toUpperCase();
  return rank === "P0" || rank === "P1" ? "danger" : "quiet";
}

/** Filled pill: background + text. `quiet`/`faint` get the neutral chip, used
 *  where the pill shape has to hold a column (agent rows, priorities). */
export const TONE_FILL: Record<StatusTone, string> = {
  live: "bg-status-running/13 text-status-running font-medium",
  attention: "bg-primary/12 text-primary font-medium",
  danger: "bg-status-failed/12 text-status-failed font-medium",
  quiet: "bg-muted text-muted-foreground",
  faint: "bg-muted text-muted-foreground opacity-70",
};

/** Bare text, no background — what a quiet status looks like when the layout
 *  does not need a chip. `live`/`attention`/`danger` keep their hue. */
export const TONE_TEXT: Record<StatusTone, string> = {
  live: "text-status-running font-medium",
  attention: "text-primary font-medium",
  danger: "text-status-failed font-medium",
  quiet: "text-muted-foreground",
  faint: "text-muted-foreground opacity-70",
};

/** Status dot. Only a live dot carries the halo; a quiet one is the flat
 *  stopped grey, which is a different token from muted text on purpose. */
export const TONE_DOT: Record<StatusTone, string> = {
  live: "bg-status-running ring-[3px] ring-status-running/18",
  attention: "bg-primary",
  danger: "bg-status-failed",
  quiet: "bg-status-stopped",
  faint: "bg-status-stopped",
};

const TASK_STATUS_LABELS: Record<string, string> = {
  open: "Open",
  in_progress: "In progress",
  wait_customer: "Wait customer",
  done: "Done",
  cancelled: "Cancelled",
};

/** `wait_customer` → `Wait customer`; anything unknown is shown as it came. */
export function taskStatusLabel(status: string): string {
  return TASK_STATUS_LABELS[status] ?? status.replace(/_/g, " ");
}

/**
 * A managed-workflow row (assignment, hold, artifact, question, observation)
 * carries a 6px dot rather than a filled row: the section is already a nested
 * block, and a second fill inside it would outrank the task's own status. The
 * rule is the same one the rest of the app uses — alive is green, needing a
 * person is red or accent — so every one of the five lists reads it from here
 * instead of deciding per list.
 */
export function workflowRowTone(state: string): StatusTone {
  const value = state.trim().toLowerCase();
  if (["running", "in_progress", "active", "claimed", "started", "leased"].includes(value)) return "live";
  if (["failed", "error", "frozen", "escalated", "timed_out", "expired"].includes(value)) return "danger";
  if (["open", "waiting", "wait_customer", "asked", "blocked", "held", "pending"].includes(value)) return "attention";
  return "quiet";
}

/** The dot itself: flat, no halo, and a quiet row shows the border tone so it
 *  holds the column without reading as a status. */
export const WORKFLOW_DOT: Record<StatusTone, string> = {
  live: "bg-status-running",
  attention: "bg-primary",
  danger: "bg-status-failed",
  quiet: "bg-border",
  faint: "bg-border",
};
