import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, expect, it, vi } from "vitest"
import TaskTransferDialog from "./TaskTransferDialog"

const registry = vi.hoisted(() => ({
  daemons: [
    { id: "local", label: "This Mac", baseURL: "" },
    { id: "builder", label: "builder", baseURL: "https://builder.test" },
  ],
  activeId: "local",
}))

vi.mock("@/components/DaemonProvider", () => ({
  useOptionalDaemons: () => ({ ...registry, appVersion: "", select: vi.fn(), refresh: vi.fn() }),
}))

beforeEach(() => {
  registry.daemons = [
    { id: "local", label: "This Mac", baseURL: "" },
    { id: "builder", label: "builder", baseURL: "https://builder.test" },
  ]
  registry.activeId = "local"
})

it("offers every server except the one the task is on, and transfers to the chosen one", async () => {
  const onTransfer = vi.fn().mockResolvedValue(undefined)
  const onOpenChange = vi.fn()
  render(<TaskTransferDialog taskKey="TEST-fkt3" queue="TEST" open
    onOpenChange={onOpenChange} onTransfer={onTransfer} />)

  const select = screen.getByLabelText("Target server")
  expect(within(select).queryByText("This Mac")).toBeNull()
  await userEvent.selectOptions(select, "builder")
  await userEvent.click(screen.getByRole("button", { name: "Move task" }))

  expect(onTransfer).toHaveBeenCalledWith("builder")
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

it("reports a refusal from the target server and keeps the dialog open", async () => {
  const onTransfer = vi.fn().mockRejectedValue(new Error("this daemon has no queue TEST"))
  const onOpenChange = vi.fn()
  render(<TaskTransferDialog taskKey="TEST-fkt3" queue="TEST" open
    onOpenChange={onOpenChange} onTransfer={onTransfer} />)

  await userEvent.selectOptions(screen.getByLabelText("Target server"), "builder")
  await userEvent.click(screen.getByRole("button", { name: "Move task" }))

  expect(await screen.findByRole("alert")).toHaveTextContent("this daemon has no queue TEST")
  expect(onOpenChange).not.toHaveBeenCalledWith(false)
})

it("says so when no other server is registered", () => {
  registry.daemons = [{ id: "local", label: "This Mac", baseURL: "" }]
  render(<TaskTransferDialog taskKey="TEST-fkt3" queue="TEST" open
    onOpenChange={vi.fn()} onTransfer={vi.fn()} />)

  expect(screen.getByText(/No other server is registered/)).toBeInTheDocument()
  expect(screen.getByRole("button", { name: "Move task" })).toBeDisabled()
})
