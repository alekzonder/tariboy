import { fireEvent, render, screen, within } from "@testing-library/react"
import { expect, it, vi } from "vitest"
import WorkflowStatusMap from "./WorkflowStatusMap"
import { agentPool, developmentManifest } from "@/test/fixtures/workflowFixtures"

const definition = developmentManifest.definition

it("lists every status in manifest order with its owner and outcomes", () => {
  render(<WorkflowStatusMap definition={definition} pools={[]} onPool={vi.fn()} />)
  const rows = screen.getAllByRole("row").slice(1)
  expect(rows.map((row) => within(row).getAllByRole("cell")[0].textContent)).toEqual(["planinitial", "approval", "review", "doneterminal"])
  expect(within(rows[1]).getByText("customer")).toBeInTheDocument()
  expect(rows[1]).toHaveTextContent("approved → review")
  expect(rows[1]).toHaveTextContent("changes_requested → plan")
})

it("shows a pool owner with its agent count and marks an empty or missing pool", () => {
  render(<WorkflowStatusMap definition={definition} pools={[agentPool("developers", ["a", "b"]), agentPool("reviewers", [])]} onPool={vi.fn()} />)
  expect(screen.getByRole("button", { name: "Pool developers, 2 agents" })).toHaveTextContent("developers · 2")
  expect(screen.getByRole("button", { name: "Pool reviewers, no agents" })).toHaveAttribute("data-empty", "true")
})

it("opens the pool a chip names", () => {
  const onPool = vi.fn()
  render(<WorkflowStatusMap definition={definition} pools={[]} onPool={onPool} />)
  fireEvent.click(screen.getByRole("button", { name: "Pool reviewers, no agents" }))
  expect(onPool).toHaveBeenCalledWith("reviewers")
})
