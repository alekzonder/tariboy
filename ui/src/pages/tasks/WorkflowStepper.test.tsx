import { fireEvent, render, screen, within } from "@testing-library/react"
import { expect, it, vi } from "vitest"
import WorkflowStepper from "./WorkflowStepper"
import { customerView } from "./workflowFixtures"

it("lists the statuses in manifest order with the current one marked", () => {
  render(<WorkflowStepper view={customerView} />)
  const steps = within(screen.getByRole("list", { name: "Workflow steps" })).getAllByRole("button")
  expect(steps.map((step) => step.textContent)).toEqual(["draft", "review", "publish", "done", "dropped"])
  expect(steps[0]).toHaveAttribute("data-state", "visited")
  expect(steps[1]).toHaveAttribute("aria-current", "step")
  expect(steps[2]).toHaveAttribute("data-state", "ahead")
  expect(screen.queryByTestId("step-detail")).not.toBeInTheDocument()
})

it("opens a step to show its owner, its gates with what each check verifies, and its limits", () => {
  render(<WorkflowStepper view={customerView} />)
  fireEvent.click(screen.getByRole("button", { name: /^draft/ }))
  const detail = screen.getByTestId("step-detail")
  expect(detail).toHaveTextContent("owned by pool writers, where a new task starts")
  expect(detail).toHaveTextContent("statuses/draft.md")
  expect(detail).toHaveTextContent("drafted")
  expect(detail).toHaveTextContent("review")
  expect(detail).toHaveTextContent("requires notes, summary")
  expect(within(detail).getByText("./checks/notes.sh")).toBeInTheDocument()
  expect(within(detail).getByText("The notes start with a heading.")).toBeInTheDocument()
  expect(detail).toHaveTextContent("as queue, timeout 30s")
  expect(detail).toHaveTextContent("pauses after 3 idle iterations, 5 rejections, or 3 script failures in a row")
  // A second click closes it; another step replaces it.
  fireEvent.click(screen.getByRole("button", { name: /^draft/ }))
  expect(screen.queryByTestId("step-detail")).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole("button", { name: /^publish/ }))
  const watch = screen.getByTestId("step-detail")
  expect(watch).toHaveTextContent("owned by a watch script")
  expect(within(watch).getByText("./scripts/ci.sh")).toBeInTheDocument()
  expect(watch).toHaveTextContent("Waits for the CI of the notes.")
  expect(watch).toHaveTextContent("every 10s, timeout 30s")
  fireEvent.click(screen.getByRole("button", { name: /^dropped/ }))
  expect(screen.getByTestId("step-detail")).toHaveTextContent("terminal, counts as cancelled")
})

it("counts how often the task came back to a status", () => {
  render(<WorkflowStepper view={{ ...customerView, visits: [
    ...customerView.visits.map((visit) => ({ ...visit, left_at: visit.left_at ?? "2026-10-01T10:05:00Z", outcome: visit.outcome ?? "rework" })),
    { id: 3, sequence: 3, status: "draft", entered_at: "2026-10-01T10:05:00Z", entered_by: "u", left_at: "2026-10-01T10:06:00Z", outcome: "drafted" },
    { id: 4, sequence: 4, status: "review", entered_at: "2026-10-01T10:06:00Z", entered_by: "u" },
  ] }} />)
  const draft = screen.getByRole("button", { name: /^draft/ })
  expect(draft).toHaveTextContent("1")
  expect(within(draft).getByTitle("The task came back to this status once")).toBeInTheDocument()
})

it("wraps and clips a long status ID while keeping it readable on hover", () => {
  const id = "s".repeat(64)
  render(<WorkflowStepper view={{ ...customerView, status: id, statuses: [{ id, owner: "customer", terminal: false }] }} />)
  expect(screen.getByRole("list", { name: "Workflow steps" })).toHaveClass("flex-wrap")
  expect(screen.getByTitle(id)).toHaveClass("truncate")
})

it("marks and opens the selected step and reports a click as the new selection", () => {
  const onSelect = vi.fn()
  render(<WorkflowStepper view={customerView} selected="review" onSelect={onSelect} />)
  const review = screen.getByRole("button", { name: /^review/ })
  expect(review).toHaveAttribute("aria-pressed", "true")
  expect(screen.getByRole("button", { name: /^draft/ })).toHaveAttribute("aria-pressed", "false")
  expect(screen.getByTestId("step-detail")).toHaveTextContent("owned by the customer")
  fireEvent.click(screen.getByRole("button", { name: /^draft/ }))
  expect(onSelect).toHaveBeenLastCalledWith("draft")
  fireEvent.click(review)
  expect(onSelect).toHaveBeenLastCalledWith(null)
  // The parent owns the selection: a click alone changes nothing.
  expect(review).toHaveAttribute("aria-pressed", "true")
})
