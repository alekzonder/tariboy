import { useState } from "react"
import { describe, it, expect, vi, beforeEach } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { PathAutocomplete, splitPath } from "./PathAutocomplete"

// Mock the api module so the component's fsList calls hit our fixture instead of
// the network. Each prefix returns a distinct listing so we can assert the
// request path and the re-list after a Tab completion.
vi.mock("@/lib/api", () => ({ fsList: vi.fn() }))
import { fsList } from "@/lib/api"
const mockFsList = vi.mocked(fsList)

const ROOT = [
  { name: "projects", dir: true },
  { name: "pictures", dir: true },
  { name: "documents", dir: true },
]
const PROJECTS = [
  { name: "alpha", dir: true },
  { name: "beta", dir: true },
]

beforeEach(() => {
  mockFsList.mockReset()
  mockFsList.mockImplementation((path: string) => {
    if (path === "") return Promise.resolve({ path: "", parent: "", entries: ROOT })
    if (path === "projects/")
      return Promise.resolve({ path: "projects", parent: "", entries: PROJECTS })
    return Promise.resolve({ path, parent: "", entries: [] })
  })
})

// Controlled harness: mirrors how the create forms will own the value. The
// visible <span> intentionally does NOT collide with dropdown queries — those
// go through role=option / role=listbox, not text.
function Harness({ initial = "" }: { initial?: string }) {
  const [v, setV] = useState(initial)
  return (
    <div>
      <PathAutocomplete value={v} onChange={setV} placeholder="cwd" aria-label="cwd" debounceMs={0} />
      <span data-testid="val">{v}</span>
    </div>
  )
}

const option = (name: string) => screen.queryByRole("option", { name })

describe("splitPath", () => {
  it("splits on the last slash, keeping the trailing slash on the prefix", () => {
    expect(splitPath("/home/u/pr")).toEqual({ prefix: "/home/u/", tail: "pr" })
    expect(splitPath("pr")).toEqual({ prefix: "", tail: "pr" })
    expect(splitPath("/home/u/")).toEqual({ prefix: "/home/u/", tail: "" })
    expect(splitPath("")).toEqual({ prefix: "", tail: "" })
  })
})

describe("PathAutocomplete", () => {
  it("typing requests the parent listing and filters it by the trailing segment", async () => {
    render(<Harness />)
    const input = screen.getByLabelText("cwd")
    await userEvent.click(input)
    await userEvent.type(input, "pr")

    // Requested the root prefix ("") and shows only the folder matching "pr".
    await waitFor(() => expect(mockFsList).toHaveBeenCalledWith("", undefined))
    expect(await screen.findByRole("option", { name: "projects" })).toBeInTheDocument()
    expect(option("documents")).not.toBeInTheDocument()
    expect(option("pictures")).not.toBeInTheDocument()
  })

  it("caches per prefix — repeated keystrokes in one segment don't refetch", async () => {
    render(<Harness />)
    const input = screen.getByLabelText("cwd")
    await userEvent.click(input)
    await userEvent.type(input, "pro")
    await screen.findByRole("option", { name: "projects" })
    expect(mockFsList.mock.calls.filter(([p]) => p === "").length).toBe(1)
  })

  it("Tab drills into the highlighted folder and re-lists its children", async () => {
    render(<Harness />)
    const input = screen.getByLabelText("cwd")
    await userEvent.click(input)
    await userEvent.type(input, "pr")
    await screen.findByRole("option", { name: "projects" })

    await userEvent.tab()

    expect(screen.getByTestId("val").textContent).toBe("projects/")
    await waitFor(() => expect(mockFsList).toHaveBeenCalledWith("projects/", undefined))
    expect(await screen.findByRole("option", { name: "alpha" })).toBeInTheDocument()
    expect(await screen.findByRole("option", { name: "beta" })).toBeInTheDocument()
  })

  it("clicking a folder completes it the same way as Tab", async () => {
    render(<Harness />)
    const input = screen.getByLabelText("cwd")
    await userEvent.click(input)
    await userEvent.type(input, "doc")
    const item = await screen.findByRole("option", { name: "documents" })

    await userEvent.click(item)

    expect(screen.getByTestId("val").textContent).toBe("documents/")
  })

  it("Enter accepts the typed path and closes the dropdown", async () => {
    render(<Harness />)
    const input = screen.getByLabelText("cwd")
    await userEvent.click(input)
    await userEvent.type(input, "projects")
    await screen.findByRole("option", { name: "projects" })

    await userEvent.keyboard("{Enter}")

    expect(screen.getByTestId("val").textContent).toBe("projects")
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument())
  })

  it("discards a late response for a superseded prefix even when the new prefix is cached", async () => {
    // Hold the projects/ listing so it can resolve AFTER we navigate back to a
    // cached prefix. Without the seq bump on the cache-hit path, this late
    // response would clobber the root listing (the stale-response race).
    let resolveProjects: (v: unknown) => void = () => {}
    const projectsDeferred = new Promise((res) => {
      resolveProjects = res
    })
    mockFsList.mockImplementation((path: string) => {
      if (path === "") return Promise.resolve({ path: "", parent: "", entries: ROOT })
      if (path === "projects/") return projectsDeferred as never
      return Promise.resolve({ path, parent: "", entries: [] })
    })

    render(<Harness initial="projects" />)
    const input = screen.getByLabelText("cwd")
    await userEvent.click(input)
    // Prime the root ("") cache.
    await screen.findByRole("option", { name: "projects" })

    // Drill into projects/: issues the deferred (still-pending) request.
    await userEvent.type(input, "/")
    await waitFor(() => expect(mockFsList).toHaveBeenCalledWith("projects/", undefined))

    // Navigate back to the cached root before projects/ resolves.
    await userEvent.clear(input)
    await screen.findByRole("option", { name: "pictures" })

    // The stale projects/ response lands now — it must be discarded, so the
    // root listing stays and the projects/ children never appear.
    resolveProjects({ path: "projects", parent: "", entries: PROJECTS })
    await waitFor(() => expect(option("projects")).toBeInTheDocument())
    expect(option("alpha")).not.toBeInTheDocument()
    expect(option("beta")).not.toBeInTheDocument()
  })

  it("passes the target daemon to fsList", async () => {
    const d = { id: "d1", label: "x", baseURL: "https://h", token: "t" }
    function DaemonHarness() {
      const [v, setV] = useState("")
      return (
        <PathAutocomplete
          value={v}
          onChange={setV}
          placeholder="cwd"
          aria-label="cwd"
          debounceMs={0}
          daemon={d}
        />
      )
    }
    render(<DaemonHarness />)
    const input = screen.getByLabelText("cwd")
    await userEvent.click(input)
    await userEvent.type(input, "pr")

    await waitFor(() => expect(mockFsList).toHaveBeenCalledWith("", d))
  })

  it("Escape closes the dropdown without changing the value", async () => {
    render(<Harness />)
    const input = screen.getByLabelText("cwd")
    await userEvent.click(input)
    await userEvent.type(input, "pr")
    await screen.findByRole("option", { name: "projects" })

    await userEvent.keyboard("{Escape}")

    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument())
    expect(screen.getByTestId("val").textContent).toBe("pr")
  })
})
