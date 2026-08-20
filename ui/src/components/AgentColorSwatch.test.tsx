import { it, expect, vi, beforeEach, afterEach } from "vitest"
import { render, screen, fireEvent, waitFor } from "@testing-library/react"
import { AgentColorSwatch } from "./AgentColorSwatch"

beforeEach(() => localStorage.clear())
afterEach(() => vi.restoreAllMocks())

// Stub fetch, recording POSTs and returning the {ok,result} envelope.
function stubFetch() {
  const calls: Array<{ path: string; method?: string; body: unknown }> = []
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((path: string, init?: RequestInit) => {
      if (init?.method)
        calls.push({
          path,
          method: init.method,
          body: init?.body ? JSON.parse(init.body as string) : undefined,
        })
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result: { name: "alpha", color: "#123abc" } }),
      } as Response)
    }),
  )
  return calls
}

it("renders the swatch with the current color as its background", () => {
  stubFetch()
  render(<AgentColorSwatch name="alpha" color="#112233" />)
  const swatch = screen.getByTestId("agent-color-swatch")
  expect(swatch.style.backgroundColor).not.toBe("")
})

it("saving a valid hex POSTs {value} to /color, caches it, and calls onSaved", async () => {
  const calls = stubFetch()
  const onSaved = vi.fn()
  render(<AgentColorSwatch name="alpha" color="#112233" onSaved={onSaved} />)

  fireEvent.click(screen.getByTestId("agent-color-swatch"))
  const input = await screen.findByLabelText("color hex")
  fireEvent.change(input, { target: { value: "#00ff00" } })
  fireEvent.click(screen.getByText("Save"))

  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.path === "/api/agents/alpha/color" &&
          c.method === "POST" &&
          (c.body as { value?: string })?.value === "#00ff00",
      ),
    ).toBe(true),
  )
  await waitFor(() => expect(onSaved).toHaveBeenCalledWith("#00ff00"))
  expect(localStorage.getItem("agent:color:alpha")).toContain("#00ff00")
})

it("disables Save for an invalid hex and does not POST", async () => {
  const calls = stubFetch()
  render(<AgentColorSwatch name="alpha" color="#112233" />)

  fireEvent.click(screen.getByTestId("agent-color-swatch"))
  const input = await screen.findByLabelText("color hex")
  fireEvent.change(input, { target: { value: "nope" } })

  const save = screen.getByText("Save").closest("button")!
  expect(save).toBeDisabled()
  expect(calls.some((c) => c.method === "POST")).toBe(false)
})

it("reveals the color wheel only after the hex input is focused", async () => {
  stubFetch()
  render(<AgentColorSwatch name="alpha" color="#112233" />)

  fireEvent.click(screen.getByTestId("agent-color-swatch"))
  const input = await screen.findByLabelText("color hex")
  expect(screen.queryByTestId("color-wheel")).toBeNull()
  fireEvent.focus(input)
  expect(screen.getByTestId("color-wheel")).toBeInTheDocument()
})
