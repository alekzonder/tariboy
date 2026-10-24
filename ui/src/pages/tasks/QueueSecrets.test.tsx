import type { ReactElement } from "react"
import { fireEvent, render as renderWith, screen, waitFor } from "@/test/queryRender"
import { beforeEach, expect, it, vi } from "vitest"
import { ApiError } from "@/lib/api"
import QueueSecrets from "./QueueSecrets"
import { remoteTarget } from "@/test/fixtures/workflowFixtures"
import { fakeApi } from "@/test/api"

const api = vi.hoisted(() => ({ listQueueSecrets: vi.fn(), setQueueSecret: vi.fn(), removeQueueSecret: vi.fn() }))
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi({ workflows: { listQueueSecrets: api.listQueueSecrets, setQueueSecret: api.setQueueSecret, removeQueueSecret: api.removeQueueSecret } }, remoteTarget)
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi })

const renderIt = () => render(<QueueSecrets queue="REL" />)
const fill = (key: string, value: string) => {
  fireEvent.change(screen.getByLabelText("Secret key"), { target: { value: key } })
  fireEvent.change(screen.getByLabelText("Secret value"), { target: { value } })
}

beforeEach(() => {
  vi.resetAllMocks()
  api.listQueueSecrets.mockResolvedValue({ secrets: [{ key: "TOKEN", updated_at: "2026-10-02T10:00:00Z" }], global_env: [] })
})

it("lists keys only and uses a password input for the value", async () => {
  renderIt()
  expect(await screen.findByText("TOKEN")).toBeInTheDocument()
  expect(screen.getByLabelText("Secret value")).toHaveAttribute("type", "password")
  expect(api.listQueueSecrets).toHaveBeenCalledWith("REL")
})

it("shows a failed load instead of claiming there are no secrets", async () => {
  api.listQueueSecrets.mockRejectedValue(new ApiError(403, "forbidden", "you may not read the secrets"))
  renderIt()
  expect(await screen.findByRole("alert")).toHaveTextContent("you may not read the secrets")
  expect(screen.queryByText("No secrets.")).not.toBeInTheDocument()
})

it("sets a secret, clears the value input, and never renders the value", async () => {
  api.setQueueSecret.mockResolvedValue({ queue: "REL", key: "NEW_KEY", updated_at: "2026-10-02T11:00:00Z" })
  api.listQueueSecrets.mockResolvedValueOnce({ secrets: [], global_env: [] }).mockResolvedValue({ secrets: [{ key: "NEW_KEY", updated_at: "2026-10-02T11:00:00Z" }], global_env: [] })
  const { container } = renderIt()
  await screen.findByLabelText("Secret key")
  fill("NEW_KEY", "s3cr3t-value")
  fireEvent.click(screen.getByRole("button", { name: "Set" }))
  await waitFor(() => expect(api.setQueueSecret).toHaveBeenCalledWith("REL", "NEW_KEY", "s3cr3t-value"))
  await screen.findByRole("button", { name: "Remove NEW_KEY" })
  expect(screen.getByLabelText("Secret value")).toHaveValue("")
  expect(container.innerHTML).not.toContain("s3cr3t-value")
})

it("refuses an invalid key before any request", async () => {
  renderIt()
  await screen.findByText("TOKEN")
  for (const key of ["1bad", "has-dash", "TARIBOY_X"]) {
    fill(key, "v")
    fireEvent.click(screen.getByRole("button", { name: "Set" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(/key/i)
  }
  expect(api.setQueueSecret).not.toHaveBeenCalled()
})

it("shows daemon errors inline without echoing the value", async () => {
  api.setQueueSecret.mockRejectedValue(new ApiError(413, "secret_too_large", "a secret value is at most 64 KiB"))
  const { container } = renderIt()
  await screen.findByText("TOKEN")
  fill("BIG", "hunter2")
  fireEvent.click(screen.getByRole("button", { name: "Set" }))
  expect(await screen.findByRole("alert")).toHaveTextContent("at most 64 KiB")
  expect(container.innerHTML).not.toContain("hunter2")
  expect(screen.getByLabelText("Secret value")).toHaveValue("")
})

it("asks before removing and does nothing when declined", async () => {
  api.removeQueueSecret.mockResolvedValue(undefined)
  renderIt()
  await screen.findByText("TOKEN")
  fireEvent.click(screen.getByRole("button", { name: "Remove TOKEN" }))
  fireEvent.click(await screen.findByRole("button", { name: "Go back" }))
  expect(api.removeQueueSecret).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Remove TOKEN" }))
  fireEvent.click(await screen.findByRole("button", { name: "Remove secret" }))
  await waitFor(() => expect(api.removeQueueSecret).toHaveBeenCalledWith("REL", "TOKEN"))
})

it("shows a refused remove inline", async () => {
  api.removeQueueSecret.mockRejectedValue(new ApiError(409, "workflow_secret_missing", "the bound workflow requires TOKEN"))
  renderIt()
  await screen.findByText("TOKEN")
  fireEvent.click(screen.getByRole("button", { name: "Remove TOKEN" }))
  fireEvent.click(await screen.findByRole("button", { name: "Remove secret" }))
  expect(await screen.findByRole("alert")).toHaveTextContent("requires TOKEN")
})

it("disables Set and every Remove while a request runs", async () => {
  let finish: () => void = () => {}
  api.removeQueueSecret.mockReturnValue(new Promise<void>((resolve) => { finish = resolve }))
  renderIt()
  await screen.findByText("TOKEN")
  fill("OTHER", "v")
  fireEvent.click(screen.getByRole("button", { name: "Remove TOKEN" }))
  fireEvent.click(await screen.findByRole("button", { name: "Remove secret" }))
  await waitFor(() => expect(screen.getByRole("button", { name: "Remove TOKEN" })).toBeDisabled())
  expect(screen.getByRole("button", { name: "Set" })).toBeDisabled()
  finish()
  await waitFor(() => expect(screen.getByRole("button", { name: "Remove TOKEN" })).toBeEnabled())
  expect(screen.getByRole("button", { name: "Set" })).toBeEnabled()
})

it("drops a needed name once that key is in the list", async () => {
  api.setQueueSecret.mockResolvedValue({ queue: "REL", key: "API_KEY", updated_at: "2026-10-02T11:00:00Z" })
  api.listQueueSecrets.mockResolvedValueOnce({ secrets: [], global_env: [] }).mockResolvedValue({ secrets: [{ key: "API_KEY", updated_at: "2026-10-02T11:00:00Z" }], global_env: [] })
  render(<QueueSecrets queue="REL" missing={["API_KEY", "OTHER"]} />)
  expect(await screen.findByRole("alert", { name: "Secrets needed" })).toHaveTextContent("API_KEY, OTHER")
  fill("API_KEY", "v")
  fireEvent.click(screen.getByRole("button", { name: "Set" }))
  await waitFor(() => expect(screen.getByRole("alert", { name: "Secrets needed" })).not.toHaveTextContent("API_KEY"))
  expect(screen.getByRole("alert", { name: "Secrets needed" })).toHaveTextContent("OTHER")
})

it("renders the names a bind needs next to the control", async () => {
  render(<QueueSecrets queue="REL" missing={["API_KEY"]} missingMessage="workflow secrets have no value: API_KEY" />)
  expect(await screen.findByRole("alert", { name: "Secrets needed" })).toHaveTextContent("API_KEY")
})

it("names required secrets the global env holds, and which a queue secret overrides", async () => {
  api.listQueueSecrets.mockResolvedValue({ secrets: [{ key: "TOKEN", updated_at: "2026-10-02T10:00:00Z" }], global_env: ["GH_TOKEN", "TOKEN"] })
  renderIt()
  const global = await screen.findByRole("list", { name: "Global env" })
  expect(global).toHaveTextContent("GH_TOKEN present in global env")
  expect(global).toHaveTextContent("TOKEN present in global env, overridden by the queue secret")
  expect(screen.getByLabelText("Secret key")).toBeEnabled()
})
