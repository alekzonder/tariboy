import { describe, expect, it } from "vitest"
import type { WorkflowView } from "@/lib/tasks"
import { canReturnToAgent, counterText, currentVisit, defaultReturnTarget, inCurrentVisit, requiredBy, returnTargets, stepArtifacts, stepMarks, stepOf } from "./workflowGraph"
import { customerView, poolView } from "./workflowFixtures"

describe("stepOf and stepMarks", () => {
  it("counts the current status from 1 in manifest order", () => {
    expect(stepOf(customerView)).toEqual({ index: 2, total: 5 })
    expect(stepOf({ ...customerView, status: "gone" })).toBeNull()
  })

  it("marks visited, current and ahead statuses, and counts returns", () => {
    const view: WorkflowView = {
      ...customerView,
      visits: [
        { id: 1, sequence: 1, status: "draft", entered_at: "2026-10-01T10:00:00Z", entered_by: "u", left_at: "2026-10-01T10:01:00Z", outcome: "drafted" },
        { id: 2, sequence: 2, status: "review", entered_at: "2026-10-01T10:01:00Z", entered_by: "u", left_at: "2026-10-01T10:02:00Z", outcome: "rework" },
        { id: 3, sequence: 3, status: "draft", entered_at: "2026-10-01T10:02:00Z", entered_by: "u", left_at: "2026-10-01T10:03:00Z", outcome: "drafted" },
        { id: 4, sequence: 4, status: "review", entered_at: "2026-10-01T10:03:00Z", entered_by: "u" },
      ],
    }
    expect(stepMarks(view).map((mark) => [mark.status.id, mark.state, mark.returns])).toEqual([
      ["draft", "visited", 1], ["review", "current", 0], ["publish", "ahead", 0], ["done", "ahead", 0], ["dropped", "ahead", 0],
    ])
  })

  it("reads only the visits of the task's current workflow version", () => {
    const view: WorkflowView = {
      ...customerView,
      status: "draft",
      visits: [
        { id: 1, sequence: 1, status: "draft", entered_at: "2026-10-01T10:00:00Z", entered_by: "u", left_at: "2026-10-01T10:01:00Z", outcome: "drafted", workflow_digest: "sha256:old" },
        { id: 2, sequence: 2, status: "review", entered_at: "2026-10-01T10:01:00Z", entered_by: "u", left_at: "2026-10-01T10:02:00Z", workflow_digest: "sha256:old" },
        { id: 3, sequence: 3, status: "draft", entered_at: "2026-10-01T10:02:00Z", entered_by: "u", workflow_digest: "sha256:abc" },
      ],
    }
    // The upgrade restarted the task: review is ahead again and draft has no return.
    expect(stepMarks(view).map((mark) => [mark.status.id, mark.state, mark.returns]).slice(0, 2)).toEqual([
      ["draft", "current", 0], ["review", "ahead", 0],
    ])
  })

  it("has no current step on a closed task", () => {
    const marks = stepMarks({ ...customerView, status: "done", category: "done" })
    expect(marks.find((mark) => mark.status.id === "done")?.state).toBe("ahead")
    expect(marks.every((mark) => mark.state !== "current")).toBe(true)
  })
})

describe("currentVisit and inCurrentVisit", () => {
  it("picks the open visit and dates things by it", () => {
    expect(currentVisit(customerView)?.id).toBe(2)
    expect(inCurrentVisit(customerView, "2026-10-01T10:05:00Z")).toBe(true)
    expect(inCurrentVisit(customerView, "2026-10-01T10:00:30Z")).toBe(false)
    expect(inCurrentVisit({ ...customerView, category: "done" }, "2026-10-01T10:05:00Z")).toBe(false)
  })
})

describe("requiredBy", () => {
  it("maps each artifact to the outcomes of the current status that need it", () => {
    const required = requiredBy(poolView)
    expect(required.get("summary")).toEqual([{ on: "drafted", to: "review", missing: true }])
    expect(required.get("notes")).toEqual([{ on: "drafted", to: "review", missing: false }])
    expect(required.has("changelog")).toBe(false)
  })
})

describe("stepArtifacts", () => {
  it("collects what every way in requires and what the step's own transitions require", () => {
    const review = stepArtifacts(customerView, "publish")
    expect([...review.enter]).toEqual([["notes", [{ from: "review", on: "approve", to: "publish", missing: false }]]])
    expect(review.leave.size).toBe(0)
    const draft = stepArtifacts(customerView, "draft")
    expect(draft.enter.size).toBe(0)
    expect([...draft.leave]).toEqual([
      ["notes", [{ from: "draft", on: "drafted", to: "review", missing: false }]],
      ["summary", [{ from: "draft", on: "drafted", to: "review", missing: true }]],
    ])
  })

  it("joins the ways in from several statuses under one artifact", () => {
    const view: WorkflowView = { ...customerView, statuses: customerView.statuses.map((status) => status.id === "publish"
      ? { ...status, transitions: [{ on: "redo", to: "review", requires: ["notes"] }] } : status) }
    expect(stepArtifacts(view, "review").enter.get("notes")).toEqual([
      { from: "draft", on: "drafted", to: "review", missing: false },
      { from: "publish", on: "redo", to: "review", missing: false },
    ])
  })

  it("reads the current status's way out from the outcomes the daemon checks", () => {
    const view: WorkflowView = { ...poolView, outcomes: [{ on: "drafted", to: "review", requires: ["summary"], missing: ["summary"] }] }
    expect([...stepArtifacts(view, "draft").leave]).toEqual([["summary", [{ from: "draft", on: "drafted", to: "review", missing: true }]]])
  })

  it("is empty for a step that requires nothing, or one the manifest does not declare", () => {
    for (const id of ["done", "gone"]) {
      const step = stepArtifacts(customerView, id)
      expect([step.enter.size, step.leave.size]).toEqual([0, 0])
    }
  })
})

describe("counterText", () => {
  it("reads the visit counter against the status limit", () => {
    const view: WorkflowView = {
      ...poolView,
      visits: [{ id: 2, sequence: 2, status: "draft", entered_at: "2026-10-01T10:04:30Z", entered_by: "u", rejected_requests: 2 }],
      statuses: poolView.statuses.map((status) => status.id === "draft"
        ? { ...status, limits: { idle_iterations: 3, rejected_requests: 5, script_failures: 3, unavailable_grace: "5m0s" } } : status),
    }
    expect(counterText(view, "rejected_requests")).toBe("2 of 5 rejections")
    expect(counterText(view, "idle_iterations")).toBe("0 of 3 idle iterations")
    const noLimits = { ...poolView, statuses: poolView.statuses.map((status) => ({ ...status, limits: undefined })) }
    expect(counterText(noLimits, "rejected_requests")).toBe("")
  })
})

describe("returnTargets", () => {
  it("offers a declared outcome to a pool status by advance in a customer status", () => {
    expect(returnTargets(customerView)).toEqual([
      { status: "draft", via: "advance", outcome: "rework", reason: "outcome" },
    ])
    expect(defaultReturnTarget(customerView, returnTargets(customerView))?.status).toBe("draft")
    expect(canReturnToAgent(customerView)).toBe(true)
  })

  it("falls back to a move of the visited pool statuses in a script status", () => {
    const view: WorkflowView = { ...customerView, status: "publish", owner: "script", waiting_on: "script", outcomes: [{ on: "published", to: "done" }],
      visits: [...customerView.visits.map((visit) => ({ ...visit, left_at: visit.left_at ?? "2026-10-01T10:06:00Z" })),
        { id: 3, sequence: 3, status: "publish", entered_at: "2026-10-01T10:06:00Z", entered_by: "u" }] }
    expect(returnTargets(view)).toEqual([{ status: "draft", via: "move", reason: "visited" }])
    expect(canReturnToAgent(view)).toBe(true)
  })

  it("does not offer a pool status visited only under an earlier workflow version", () => {
    const view: WorkflowView = { ...customerView, status: "publish", owner: "script", waiting_on: "script", outcomes: [{ on: "published", to: "done" }],
      visits: [
        { id: 1, sequence: 1, status: "draft", entered_at: "2026-10-01T10:00:00Z", entered_by: "u", left_at: "2026-10-01T10:01:00Z", workflow_digest: "sha256:old" },
        { id: 2, sequence: 2, status: "publish", entered_at: "2026-10-01T10:01:00Z", entered_by: "u", workflow_digest: "sha256:abc" },
      ] }
    expect(returnTargets(view)).toEqual([])
  })

  it("offers the status itself to a paused pool status, and uses a move for its outcomes", () => {
    const view: WorkflowView = { ...poolView, waiting_on: "pause", paused_reason: "idle_iterations", category: "wait_customer",
      outcomes: [{ on: "drafted", to: "review" }, { on: "back", to: "draft" }] }
    expect(returnTargets(view)).toEqual([{ status: "draft", via: "move", reason: "outcome" }])
    expect(canReturnToAgent(view)).toBe(true)
  })

  it("is hidden for an unpaused pool status with a holder and for a closed task", () => {
    expect(canReturnToAgent(poolView)).toBe(false)
    expect(returnTargets({ ...customerView, category: "done" })).toEqual([])
    expect(canReturnToAgent({ ...customerView, category: "done" })).toBe(false)
  })

  it("prefers the most recently visited pool status as the default", () => {
    const view: WorkflowView = { ...customerView,
      statuses: [{ id: "plan", owner: "pool:planners", terminal: false }, ...customerView.statuses],
      outcomes: [{ on: "replan", to: "plan" }, { on: "rework", to: "draft" }],
      visits: [{ id: 0, sequence: 0, status: "plan", entered_at: "2026-10-01T09:00:00Z", entered_by: "u", left_at: "2026-10-01T10:00:00Z" }, ...customerView.visits] }
    const targets = returnTargets(view)
    expect(targets.map((target) => target.status)).toEqual(["plan", "draft"])
    expect(defaultReturnTarget(view, targets)?.status).toBe("draft")
  })
})
