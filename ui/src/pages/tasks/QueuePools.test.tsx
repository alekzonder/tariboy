import type { ReactElement } from "react"
import { fireEvent, render as renderWith, screen, waitFor, within } from "@/test/queryRender"
import { beforeEach, expect, it, vi } from "vitest"
import { ApiError } from "@/lib/api"
import QueuePools from "./QueuePools"
import { agentPool, remoteTarget } from "./workflowFixtures"
import { fakeApi } from "@/test/api"

const api = vi.hoisted(() => ({ rebindAgentPool: vi.fn() }))
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi({ tasks: { rebindAgentPool: api.rebindAgentPool } }, remoteTarget)
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi })
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const required = new Map([["developers", ["plan", "implement"]], ["reviewers", ["review"]]])
const renderIt = (props: Partial<Parameters<typeof QueuePools>[0]> = {}) => {
  const onSaved = vi.fn()
  const onReload = vi.fn()
  render(<QueuePools queue="DEV" pools={[agentPool("developers", ["dev-a"], 4)]}
    required={required} agents={["dev-a", "dev-b", "rev-a"]} onSaved={onSaved} onReload={onReload} {...props} />)
  return { onSaved, onReload }
}

beforeEach(() => { vi.resetAllMocks() })

it("lists a pool the workflow requires before it exists, with the statuses that use it", () => {
  renderIt()
  const reviewers = screen.getByRole("group", { name: "Pool reviewers" })
  expect(reviewers).toHaveTextContent("review")
  expect(reviewers).toHaveTextContent("no agents")
  expect(screen.getByRole("group", { name: "Pool developers" })).toHaveTextContent("plan, implement")
})

it("adds an agent and saves the full list with revision 0 for a new pool", async () => {
  api.rebindAgentPool.mockResolvedValue(agentPool("reviewers", ["rev-a"], 1))
  const { onSaved } = renderIt()
  const reviewers = screen.getByRole("group", { name: "Pool reviewers" })
  expect(within(reviewers).getByRole("button", { name: "Save pool reviewers" })).toBeDisabled()
  fireEvent.change(within(reviewers).getByLabelText("Add agent to reviewers"), { target: { value: "rev-a" } })
  fireEvent.click(within(reviewers).getByRole("button", { name: "Add" }))
  expect(within(reviewers).getByText("rev-a")).toBeInTheDocument()
  fireEvent.click(within(reviewers).getByRole("button", { name: "Save pool reviewers" }))
  await waitFor(() => expect(api.rebindAgentPool).toHaveBeenCalledWith("DEV", "reviewers", ["rev-a"], 0, expect.any(String)))
  expect(onSaved).toHaveBeenCalledWith(agentPool("reviewers", ["rev-a"], 1))
})

it("removes an agent and saves with the pool's revision", async () => {
  api.rebindAgentPool.mockResolvedValue(agentPool("developers", ["dev-a", "dev-b"], 5))
  renderIt({ pools: [agentPool("developers", ["dev-a", "dev-b", "old"], 4)] })
  const developers = screen.getByRole("group", { name: "Pool developers" })
  fireEvent.click(within(developers).getByRole("button", { name: "Remove old from developers" }))
  fireEvent.click(within(developers).getByRole("button", { name: "Save pool developers" }))
  await waitFor(() => expect(api.rebindAgentPool).toHaveBeenCalledWith("DEV", "developers", ["dev-a", "dev-b"], 4, expect.any(String)))
})

it("reloads and says so when the pool changed elsewhere", async () => {
  api.rebindAgentPool.mockRejectedValue(new ApiError(409, "revision_conflict", "revision conflict"))
  const { onReload } = renderIt()
  const developers = screen.getByRole("group", { name: "Pool developers" })
  fireEvent.change(within(developers).getByLabelText("Add agent to developers"), { target: { value: "dev-b" } })
  fireEvent.click(within(developers).getByRole("button", { name: "Add" }))
  fireEvent.click(within(developers).getByRole("button", { name: "Save pool developers" }))
  expect(await screen.findByText(/changed elsewhere/)).toBeInTheDocument()
  expect(onReload).toHaveBeenCalled()
})

it("shows a refused save on its row and keeps the edit", async () => {
  api.rebindAgentPool.mockRejectedValue(new ApiError(409, "workflow_pool_empty", "pool developers is used by the bound workflow"))
  renderIt()
  const developers = screen.getByRole("group", { name: "Pool developers" })
  fireEvent.click(within(developers).getByRole("button", { name: "Remove dev-a from developers" }))
  fireEvent.click(within(developers).getByRole("button", { name: "Save pool developers" }))
  expect(await within(developers).findByRole("alert")).toHaveTextContent("used by the bound workflow")
  expect(within(developers).queryByText("dev-a")).not.toBeInTheDocument()
  expect(within(developers).getByRole("button", { name: "Save pool developers" })).toBeEnabled()
})

it("adds a row for a new pool by name", () => {
  renderIt()
  fireEvent.click(screen.getByRole("button", { name: "Add pool" }))
  fireEvent.change(screen.getByLabelText("New pool name"), { target: { value: "qa" } })
  fireEvent.click(screen.getByRole("button", { name: "Create pool row" }))
  expect(screen.getByRole("group", { name: "Pool qa" })).toBeInTheDocument()
})

it("suggests the server's agents", () => {
  renderIt()
  const input = screen.getByLabelText("Add agent to developers")
  const list = document.getElementById(input.getAttribute("list")!)
  expect([...list!.querySelectorAll("option")].map((option) => option.value)).toEqual(["dev-a", "dev-b", "rev-a"])
})
