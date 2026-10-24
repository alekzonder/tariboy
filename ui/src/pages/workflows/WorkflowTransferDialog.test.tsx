import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"
import { ApiError, createApi, type ApiClient, type WorkflowBuildResult } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"
import { apiWrapper } from "@/test/api"
import { WorkflowTransferDialog } from "./WorkflowTransferDialog"

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>()
  return { ...actual, createApi: vi.fn(actual.createApi) }
})
const actualApi = await vi.importActual<typeof import("@/lib/api")>("@/lib/api")

// Each archive call records the host of the client that made it.
const download = vi.fn<(target: Daemon | null, name: string, tag: string) => Promise<Blob>>()
const upload = vi.fn<(target: Daemon | null, archive: Blob) => Promise<WorkflowBuildResult>>()
function archiveClient(target: Daemon | null): ApiClient {
  const client = actualApi.createApi(target)
  return {
    ...client,
    workflows: {
      ...client.workflows,
      downloadWorkflowArchive: (name: string, tag: string) => download(target, name, tag),
      importWorkflowArchive: (archive: Blob) => upload(target, archive),
    },
  }
}

beforeEach(() => { vi.mocked(createApi).mockImplementation((target) => archiveClient(actualApi.resolveTarget(target))) })
afterEach(() => vi.resetAllMocks())

const source = { id: "source", label: "Source", baseURL: "https://source", state: "ready", token: "s" } as const
const a = { id: "a", label: "Alpha", baseURL: "https://a", state: "ready", token: "a" } as const
const b = { id: "b", label: "Beta", baseURL: "https://b", state: "ready", token: "b" } as const
const c = { id: "c", label: "Gamma", baseURL: "https://c", state: "ready", token: "c" } as const

const result = (created: boolean) => ({ name: "dev", version: "1.0.0", digest: "d", tags: ["1.0.0", "latest"], created })

function open(onComplete = vi.fn()) {
  render(<WorkflowTransferDialog open onOpenChange={() => undefined} name="dev" tag="1.0.0"
    daemons={[source, a, b, c]} onComplete={onComplete} />, { wrapper: apiWrapper(archiveClient(source)) })
  return onComplete
}

it("offers every ready server but the source", () => {
  open()
  expect(screen.getByRole("checkbox", { name: "Transfer to This daemon (local)" })).toBeInTheDocument()
  expect(screen.getByRole("checkbox", { name: "Transfer to Alpha" })).toBeInTheDocument()
  expect(screen.queryByRole("checkbox", { name: "Transfer to Source" })).not.toBeInTheDocument()
  expect(screen.getByText(/latest/)).toBeInTheDocument()
})

it("exports once and imports on each selected server, continuing past a failure", async () => {
  const archive = new Blob(["archive"])
  download.mockResolvedValue(archive)
  upload.mockImplementation(async (target) => {
    if (target === b) throw new ApiError(409, "workflow_version_published", "dev 1.0.0 is taken")
    return result(target !== c)
  })
  const onComplete = open()
  for (const label of ["Alpha", "Beta", "Gamma"]) fireEvent.click(screen.getByRole("checkbox", { name: `Transfer to ${label}` }))
  fireEvent.click(screen.getByRole("button", { name: "Start transfer" }))
  await waitFor(() => expect(onComplete).toHaveBeenCalled())
  expect(download).toHaveBeenCalledTimes(1)
  expect(download).toHaveBeenCalledWith(source, "dev", "1.0.0")
  expect(upload.mock.calls).toEqual([[a, archive], [b, archive], [c, archive]])
  const progress = screen.getByRole("list", { name: "Transfer progress" })
  expect(progress).toHaveTextContent("Alpha: Completed")
  expect(progress).toHaveTextContent("Beta: Failed — dev 1.0.0 is taken")
  expect(progress).toHaveTextContent("Gamma: Already present")
})

it("cancels the servers not yet started", async () => {
  download.mockResolvedValue(new Blob(["archive"]))
  let finish: (value: ReturnType<typeof result>) => void = () => undefined
  upload.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
  open()
  fireEvent.click(screen.getByRole("checkbox", { name: "Transfer to Alpha" }))
  fireEvent.click(screen.getByRole("checkbox", { name: "Transfer to Beta" }))
  fireEvent.click(screen.getByRole("button", { name: "Start transfer" }))
  await waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
  fireEvent.click(screen.getByRole("button", { name: "Cancel transfer" }))
  finish(result(true))
  const progress = screen.getByRole("list", { name: "Transfer progress" })
  await waitFor(() => expect(progress).toHaveTextContent("Beta: Cancelled"))
  expect(progress).toHaveTextContent("Alpha: Completed")
  expect(upload).toHaveBeenCalledTimes(1)
})

it("fails every row when the export fails", async () => {
  download.mockRejectedValue(new Error("export broke"))
  open()
  fireEvent.click(screen.getByRole("checkbox", { name: "Transfer to Alpha" }))
  fireEvent.click(screen.getByRole("button", { name: "Start transfer" }))
  expect(await screen.findByText("Alpha: Failed — export broke")).toBeInTheDocument()
  expect(upload).not.toHaveBeenCalled()
})
