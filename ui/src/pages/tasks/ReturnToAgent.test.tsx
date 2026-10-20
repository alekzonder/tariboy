import { act, render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, expect, it, vi } from "vitest"
import { ApiError } from "@/lib/api"
import type { TransitionRequest, WorkflowView } from "@/lib/tasks"
import ReturnToAgent from "./ReturnToAgent"
import { customerView, poolView, remoteTarget, workflowTask } from "./workflowFixtures"

const api = vi.hoisted(() => ({ advanceTask: vi.fn(), moveTaskWorkflow: vi.fn(), getTransitionRequest: vi.fn() }))
vi.mock("@/lib/tasks", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/tasks")>(),
  ...api,
}))

const target = remoteTarget
const applied: TransitionRequest = { id: 11, task_key: "REL-1", outcome: "rework", actor: "user:owner", state: "applied", created_at: "" }
/** The task in the script status `publish`, after draft and review. */
const scriptView: WorkflowView = { ...customerView, status: "publish", owner: "script", waiting_on: "script", outcomes: [{ on: "published", to: "done" }],
  visits: [...customerView.visits.map((visit) => ({ ...visit, left_at: visit.left_at ?? "2026-10-01T10:06:00Z" })),
    { id: 3, sequence: 3, status: "publish", entered_at: "2026-10-01T10:06:00Z", entered_by: "u" }] }

beforeEach(() => {
  vi.clearAllMocks()
  api.moveTaskWorkflow.mockResolvedValue(workflowTask)
})

function renderReturn(view: WorkflowView) {
  const onChanged = vi.fn()
  const onDone = vi.fn()
  render(<ReturnToAgent taskKey="REL-1" view={view} target={target} onChanged={onChanged} onDone={onDone} />)
  return { onChanged, onDone }
}

async function sendMessage(text: string) {
  await userEvent.type(screen.getByLabelText("Message to the agent"), text)
  await userEvent.click(screen.getByRole("button", { name: "Return to agent" }))
  const dialog = await screen.findByRole("alertdialog")
  await userEvent.click(within(dialog).getByRole("button", { name: "Return to agent" }))
}

it("declares the outcome of a customer status with the message, and closes once it applies", async () => {
  api.advanceTask.mockResolvedValue(applied)
  const { onChanged, onDone } = renderReturn(customerView)
  const select = screen.getByLabelText("Return to status")
  expect(select).toHaveValue("draft")
  expect(select).toHaveFocus()
  expect(screen.getByText("declares rework")).toBeInTheDocument()
  expect(screen.getByRole("button", { name: "Return to agent" })).toBeDisabled()
  await sendMessage("Please add a heading")
  await waitFor(() => expect(api.advanceTask).toHaveBeenCalledWith("REL-1", "rework", "Please add a heading", "review", target))
  expect(api.moveTaskWorkflow).not.toHaveBeenCalled()
  await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(1))
  expect(onDone).toHaveBeenCalledTimes(1)
})

it("moves the task back to the pool status it visited when no outcome leads there", async () => {
  const { onChanged, onDone } = renderReturn(scriptView)
  const select = screen.getByLabelText("Return to status")
  expect(select).toHaveValue("draft")
  expect(select.querySelector("optgroup")).toHaveAttribute("label", "By move")
  expect(screen.getByText("operator move")).toBeInTheDocument()
  await sendMessage("CI is red on the branch")
  await waitFor(() => expect(api.moveTaskWorkflow).toHaveBeenCalledWith("REL-1", "draft", "CI is red on the branch", target))
  expect(api.advanceTask).not.toHaveBeenCalled()
  await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
  expect(onChanged).toHaveBeenCalledTimes(1)
})

it("offers a paused pool status itself as a restart, by move", async () => {
  renderReturn({ ...poolView, waiting_on: "pause", paused_reason: "idle_iterations", category: "wait_customer" })
  const options = within(screen.getByLabelText("Return to status")).getAllByRole("option")
  expect(options.map((option) => option.textContent)).toEqual(["draft (restart this step)"])
  await sendMessage("Start over from the plan")
  await waitFor(() => expect(api.moveTaskWorkflow).toHaveBeenCalledWith("REL-1", "draft", "Start over from the plan", target))
})

it("keeps the form open and shows the refusal when the daemon says no", async () => {
  api.moveTaskWorkflow.mockRejectedValue(new ApiError(409, "status_unknown", "status draft is not declared"))
  const { onDone } = renderReturn(scriptView)
  await sendMessage("back")
  expect(await screen.findByRole("alert")).toHaveTextContent("status draft is not declared")
  expect(onDone).not.toHaveBeenCalled()
})

it("reports an outcome that settled without applying and refetches", async () => {
  api.advanceTask.mockResolvedValue({ ...applied, state: "rejected", result_message: "the notes are empty" })
  const { onChanged, onDone } = renderReturn(customerView)
  await sendMessage("redo")
  expect(await screen.findByRole("alert")).toHaveTextContent("rejected: the notes are empty")
  expect(onChanged).toHaveBeenCalled()
  expect(onDone).not.toHaveBeenCalled()
})

it("does nothing when the confirmation is declined, and closes on Close", async () => {
  const { onDone } = renderReturn(customerView)
  await userEvent.type(screen.getByLabelText("Message to the agent"), "x")
  await userEvent.click(screen.getByRole("button", { name: "Return to agent" }))
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Go back" }))
  await act(async () => {})
  expect(api.advanceTask).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole("button", { name: "Close" }))
  expect(onDone).toHaveBeenCalledTimes(1)
})
