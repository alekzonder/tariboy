import type { WorkflowStatusInfo, WorkflowView, WorkflowVisit } from "@/lib/tasks"

/**
 * Pure readings of a workflow view: where the task is in its status graph,
 * what the current status needs, and where it could be sent back to. The
 * components render these; nothing here touches the API.
 */

/** Whether the task is done or cancelled: no owner, no exit. */
export function isClosed(view: WorkflowView): boolean {
  return view.category === "done" || view.category === "cancelled"
}

export function isPoolStatus(status: WorkflowStatusInfo | undefined): boolean {
  return !!status && status.owner.startsWith("pool:")
}

/** The status the view is in, as the manifest declares it. */
export function currentStatus(view: WorkflowView): WorkflowStatusInfo | undefined {
  return view.statuses.find((status) => status.id === view.status)
}

/** The open visit: the stay in the current status, with its counters. */
export function currentVisit(view: WorkflowView): WorkflowVisit | undefined {
  return [...view.visits].filter((visit) => !visit.left_at).sort((a, b) => b.sequence - a.sequence)[0]
}

/** Whether a request, run, or event at `at` belongs to the open visit of an open task. */
export function inCurrentVisit(view: WorkflowView, at: string): boolean {
  if (isClosed(view)) return false
  const open = currentVisit(view)
  return !!open && Date.parse(at) >= Date.parse(open.entered_at)
}

/** The position of the current status in manifest order, from 1. */
export function stepOf(view: WorkflowView): { index: number; total: number } | null {
  const index = view.statuses.findIndex((status) => status.id === view.status)
  return index < 0 ? null : { index: index + 1, total: view.statuses.length }
}

export interface StepMark {
  status: WorkflowStatusInfo
  /** `current` is where the task is; `visited` was entered before; `ahead` not yet. */
  state: "current" | "visited" | "ahead"
  /** How many times the task came back to this status from a later one. */
  returns: number
}

/**
 * Every status in manifest order with how the task has moved through it. A
 * return is a visit whose status comes earlier in the manifest than the
 * visit before it: the loop a `changes_requested` draws.
 */
export function stepMarks(view: WorkflowView): StepMark[] {
  const order = new Map(view.statuses.map((status, index) => [status.id, index]))
  const visited = new Set<string>()
  const returns = new Map<string, number>()
  const visits = [...view.visits].sort((a, b) => a.sequence - b.sequence)
  visits.forEach((visit, i) => {
    visited.add(visit.status)
    const before = visits[i - 1]
    if (before && (order.get(visit.status) ?? -1) < (order.get(before.status) ?? -1)) {
      returns.set(visit.status, (returns.get(visit.status) ?? 0) + 1)
    }
  })
  const closed = isClosed(view)
  return view.statuses.map((status) => ({
    status,
    state: status.id === view.status && !closed ? "current" : visited.has(status.id) ? "visited" : "ahead",
    returns: returns.get(status.id) ?? 0,
  }))
}

export interface Requirement {
  /** The status the transition leaves; absent where it is the current one. */
  from?: string
  on: string
  to: string
  missing: boolean
}

export interface StepArtifacts {
  /** For each artifact name, the transitions into the step that require it. */
  enter: Map<string, Requirement[]>
  /** For each artifact name, the transitions out of the step that require it. */
  leave: Map<string, Requirement[]>
}

/**
 * What a step of the workflow needs: the artifacts a transition into it
 * requires, and those a transition out of it requires. The current status of
 * an open task reads its way out from the view's outcomes, as the daemon
 * checks them; any other step reads the manifest's transitions, and an
 * artifact is missing there when it has no value.
 */
export function stepArtifacts(view: WorkflowView, statusId: string): StepArtifacts {
  const set = new Set(view.artifacts.map((artifact) => artifact.name))
  const add = (map: Map<string, Requirement[]>, name: string, requirement: Requirement) =>
    map.set(name, [...(map.get(name) ?? []), requirement])
  const enter = new Map<string, Requirement[]>()
  for (const status of view.statuses) {
    for (const transition of status.transitions ?? []) {
      if (transition.to !== statusId) continue
      for (const name of transition.requires ?? []) {
        add(enter, name, { from: status.id, on: transition.on, to: transition.to, missing: !set.has(name) })
      }
    }
  }
  const leave = new Map<string, Requirement[]>()
  if (statusId === view.status && !isClosed(view)) {
    for (const outcome of view.outcomes) {
      const missing = new Set(outcome.missing ?? [])
      for (const name of outcome.requires ?? []) {
        add(leave, name, { from: statusId, on: outcome.on, to: outcome.to, missing: missing.has(name) })
      }
    }
  } else {
    for (const transition of view.statuses.find((status) => status.id === statusId)?.transitions ?? []) {
      for (const name of transition.requires ?? []) {
        add(leave, name, { from: statusId, on: transition.on, to: transition.to, missing: !set.has(name) })
      }
    }
  }
  return { enter, leave }
}

/** For each artifact name, the outcomes of the current status that require it. */
export function requiredBy(view: WorkflowView): Map<string, Requirement[]> {
  const out = new Map<string, Requirement[]>()
  for (const outcome of view.outcomes) {
    const missing = new Set(outcome.missing ?? [])
    for (const name of outcome.requires ?? []) {
      const list = out.get(name) ?? []
      list.push({ on: outcome.on, to: outcome.to, missing: missing.has(name) })
      out.set(name, list)
    }
  }
  return out
}

/** `used of limit noun`, or "" when the view does not carry the counter or limit. */
export function counterText(view: WorkflowView, counter: "rejected_requests" | "script_failures" | "idle_iterations"): string {
  const used = currentVisit(view)?.[counter] ?? 0
  const limit = currentStatus(view)?.limits?.[counter]
  if (!limit) return ""
  const noun = counter === "rejected_requests" ? "rejections" : counter === "script_failures" ? "script failures" : "idle iterations"
  return `${used} of ${limit} ${noun}`
}

export interface ReturnTarget {
  /** The pool status the task goes back to. */
  status: string
  /** `advance` declares the outcome; `move` is the operator's override with the message as its reason. */
  via: "advance" | "move"
  outcome?: string
  /** Why this target is offered. */
  reason: "outcome" | "visited" | "restart"
}

/**
 * Where "Return to agent" can send the task: the pool statuses a declared
 * outcome of the current status leads to, then the pool statuses the task
 * visited before, newest first, and, for a paused pool status, the status
 * itself. An outcome is used only where the customer may declare it; a move
 * is the way out of a script status, a pool status, or a pause.
 */
export function returnTargets(view: WorkflowView): ReturnTarget[] {
  if (isClosed(view)) return []
  const pools = new Map(view.statuses.filter(isPoolStatus).map((status) => [status.id, status]))
  const paused = view.waiting_on === "pause"
  const out: ReturnTarget[] = []
  const seen = new Set<string>()
  for (const outcome of view.outcomes) {
    if (!pools.has(outcome.to) || seen.has(outcome.to)) continue
    seen.add(outcome.to)
    const declared = view.owner === "customer" && !paused
    out.push({ status: outcome.to, via: declared ? "advance" : "move", outcome: declared ? outcome.on : undefined, reason: "outcome" })
  }
  const visits = [...view.visits].sort((a, b) => b.sequence - a.sequence)
  for (const visit of visits) {
    if (visit.status === view.status || !pools.has(visit.status) || seen.has(visit.status)) continue
    seen.add(visit.status)
    out.push({ status: visit.status, via: "move", reason: "visited" })
  }
  if (paused && pools.has(view.status) && !seen.has(view.status)) {
    out.push({ status: view.status, via: "move", reason: "restart" })
  }
  return out
}

/**
 * The target offered first: the pool status the task visited most recently,
 * which is where the work was done; otherwise the first declared one.
 */
export function defaultReturnTarget(view: WorkflowView, targets: ReturnTarget[]): ReturnTarget | undefined {
  const visits = [...view.visits].sort((a, b) => b.sequence - a.sequence)
  for (const visit of visits) {
    const target = targets.find((candidate) => candidate.status === visit.status && visit.status !== view.status)
    if (target) return target
  }
  return targets[0]
}

/**
 * Whether the panel offers "Return to agent" at all. An unpaused pool status
 * with a holder already has the agent on it, so the action would say nothing.
 */
export function canReturnToAgent(view: WorkflowView): boolean {
  if (isClosed(view)) return false
  if (view.owner.startsWith("pool:") && view.holder && view.waiting_on !== "pause") return false
  return returnTargets(view).length > 0
}
