import { describe, expect, it, vi } from "vitest"
import { act, render, screen } from "@testing-library/react"
import { MarkdownContent, MarkdownEditor } from "./TaskMarkdown"

// The TipTap editor is its own module; a viewer must not wait for it.
const gate = vi.hoisted(() => {
  let release!: () => void
  const promise = new Promise<void>((resolve) => {
    release = resolve
    setTimeout(resolve, 10_000)
  })
  return { promise, release }
})

vi.mock("./TaskMarkdownEditor", async () => {
  await gate.promise
  return { default: ({ placeholder }: { placeholder?: string }) => <div role="textbox" aria-label={placeholder} /> }
})

describe("TaskMarkdown loading", () => {
  it("renders Markdown without the editor module", () => {
    render(<MarkdownContent>{"**done**"}</MarkdownContent>)
    expect(screen.getByText("done").tagName).toBe("STRONG")
  })

  it("shows a loading status until the rich text editor loads", async () => {
    render(<MarkdownEditor value="" onChange={() => {}} placeholder="Description" />)
    expect(screen.getByText("Loading…", { selector: "[role=status]" })).toBeInTheDocument()

    await act(async () => gate.release())
    expect(await screen.findByRole("textbox", { name: "Description" })).toBeInTheDocument()
  })
})
