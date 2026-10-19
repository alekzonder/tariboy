import { fireEvent, render, screen, waitFor, within } from "@testing-library/react"
import { beforeEach, expect, it, vi } from "vitest"
import { ApiError } from "@/lib/api"
import QueueEnvironment from "./QueueEnvironment"
import { remoteTarget } from "./workflowFixtures"

const api = vi.hoisted(() => ({
  listQueueEnv: vi.fn(), setQueueEnv: vi.fn(), removeQueueEnv: vi.fn(),
  listQueueSecrets: vi.fn(), setQueueSecret: vi.fn(), removeQueueSecret: vi.fn(),
}))
vi.mock("@/lib/tasks", async (importOriginal) => ({ ...await importOriginal<typeof import("@/lib/tasks")>(), ...api }))

const env = () => screen.getByRole("region", { name: "Env REL" })
const fillEnv = (key: string, value: string) => {
  fireEvent.change(screen.getByLabelText("Env key"), { target: { value: key } })
  fireEvent.change(screen.getByLabelText("Env value"), { target: { value } })
}

beforeEach(() => {
  vi.resetAllMocks()
  api.listQueueEnv.mockResolvedValue([{ key: "LOG_LEVEL", value: "debug", updated_at: "2026-10-02T10:00:00Z" }])
  api.listQueueSecrets.mockResolvedValue({ secrets: [{ key: "TOKEN", updated_at: "2026-10-02T10:00:00Z" }], global_env: [] })
})

it("shows env vars with their values apart from the secrets", async () => {
  render(<QueueEnvironment queue="REL" target={remoteTarget} />)
  expect(await within(env()).findByText("LOG_LEVEL")).toBeInTheDocument()
  expect(within(env()).getByText("debug")).toBeInTheDocument()
  expect(within(env()).queryByText("TOKEN")).not.toBeInTheDocument()
  expect(within(screen.getByRole("region", { name: "Secrets REL" })).getByText("TOKEN")).toBeInTheDocument()
  expect(screen.getByLabelText("Env value")).not.toHaveAttribute("type", "password")
})

it("sets an env var and reloads both blocks", async () => {
  api.setQueueEnv.mockResolvedValue({ queue: "REL", key: "BASE_URL", value: "https://x.test", updated_at: "t" })
  render(<QueueEnvironment queue="REL" target={remoteTarget} />)
  await within(env()).findByText("LOG_LEVEL")
  fillEnv("BASE_URL", "https://x.test")
  fireEvent.click(within(env()).getByRole("button", { name: "Set" }))
  await waitFor(() => expect(api.setQueueEnv).toHaveBeenCalledWith("REL", "BASE_URL", "https://x.test", remoteTarget))
  await waitFor(() => expect(api.listQueueSecrets).toHaveBeenCalledTimes(2))
  expect(api.listQueueEnv).toHaveBeenCalledTimes(2)
  expect(screen.getByLabelText("Env key")).toHaveValue("")
})

it("asks before moving a secret key into env", async () => {
  api.setQueueEnv.mockResolvedValue({ queue: "REL", key: "TOKEN", value: "plain", updated_at: "t" })
  render(<QueueEnvironment queue="REL" target={remoteTarget} />)
  await screen.findByText("TOKEN")
  fillEnv("TOKEN", "plain")
  fireEvent.click(within(env()).getByRole("button", { name: "Set" }))
  fireEvent.click(await screen.findByRole("button", { name: "Go back" }))
  expect(api.setQueueEnv).not.toHaveBeenCalled()
  fireEvent.click(within(env()).getByRole("button", { name: "Set" }))
  fireEvent.click(await screen.findByRole("button", { name: "Move to env" }))
  await waitFor(() => expect(api.setQueueEnv).toHaveBeenCalledWith("REL", "TOKEN", "plain", remoteTarget))
})

it("asks before moving an env key into secrets", async () => {
  api.setQueueSecret.mockResolvedValue({ queue: "REL", key: "LOG_LEVEL", updated_at: "t" })
  render(<QueueEnvironment queue="REL" target={remoteTarget} />)
  await screen.findByText("LOG_LEVEL")
  fireEvent.change(screen.getByLabelText("Secret key"), { target: { value: "LOG_LEVEL" } })
  fireEvent.change(screen.getByLabelText("Secret value"), { target: { value: "s3cr3t" } })
  fireEvent.click(within(screen.getByRole("region", { name: "Secrets REL" })).getByRole("button", { name: "Set" }))
  fireEvent.click(await screen.findByRole("button", { name: "Move to secrets" }))
  await waitFor(() => expect(api.setQueueSecret).toHaveBeenCalledWith("REL", "LOG_LEVEL", "s3cr3t", remoteTarget))
  expect(screen.getByLabelText("Secret value")).toHaveValue("")
})

it("removes an env var after a confirmation and shows a refusal inline", async () => {
  api.removeQueueEnv.mockRejectedValue(new ApiError(409, "workflow_secret_missing", "the bound workflow requires LOG_LEVEL"))
  render(<QueueEnvironment queue="REL" target={remoteTarget} />)
  await screen.findByText("LOG_LEVEL")
  fireEvent.click(screen.getByRole("button", { name: "Remove LOG_LEVEL" }))
  fireEvent.click(await screen.findByRole("button", { name: "Remove env var" }))
  await waitFor(() => expect(api.removeQueueEnv).toHaveBeenCalledWith("REL", "LOG_LEVEL", remoteTarget))
  expect(await within(env()).findByRole("alert")).toHaveTextContent("requires LOG_LEVEL")
})

it("counts an env var as a value for a name a bind needs", async () => {
  render(<QueueEnvironment queue="REL" target={remoteTarget} missing={["LOG_LEVEL", "OTHER"]} />)
  await screen.findByText("LOG_LEVEL")
  await waitFor(() => expect(screen.getByRole("alert", { name: "Secrets needed" })).not.toHaveTextContent("LOG_LEVEL"))
  expect(screen.getByRole("alert", { name: "Secrets needed" })).toHaveTextContent("OTHER")
})
