import { useState } from "react"
import { act, fireEvent, render, screen } from "@testing-library/react"
import type { Editor } from "@tiptap/react"
import { describe, expect, it, vi } from "vitest"
import { MarkdownContent, MarkdownEditor } from "./TaskMarkdown"

describe("task Markdown", () => {
  it("renders task formatting and neutralizes executable markup", () => {
    const { container } = render(<MarkdownContent>{"# Plan\n\n**Bold** and ~~removed~~\n\n- [x] Done\n\n| Name | State |\n| --- | --- |\n| Work | Ready |\n\n[Unsafe](javascript:alert%281%29)\n\n<script>alert(1)</script>"}</MarkdownContent>)
    expect(screen.getByRole("heading", { name: "Plan" })).toBeInTheDocument()
    expect(container.querySelector("strong")).toHaveTextContent("Bold")
    expect(container.querySelector("del")).toHaveTextContent("removed")
    expect(screen.getByRole("checkbox")).toBeChecked()
    expect(screen.getByRole("table")).toHaveTextContent("Ready")
    expect(container.querySelector("script")).toBeNull()
    expect(container.querySelector('a[href^="javascript:"]')).toBeNull()
  })

  it("keeps unsupported Markdown verbatim in source mode", () => {
    const value = "# Plan\n\n<!-- keep this -->\n\nA footnote[^1].\n\n[^1]: preserve me\n"
    const onChange = vi.fn()
    render(<MarkdownEditor id="description" value={value} onChange={onChange} />)
    expect(screen.getByRole("textbox")).toHaveValue(value)
    expect(screen.getByRole("button", { name: "Rich text" })).toBeDisabled()
    expect(screen.getByRole("status")).toHaveTextContent(/preserv/i)
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.change(screen.getByRole("textbox"), { target: { value: value + "More" } })
    expect(onChange).toHaveBeenCalledWith(value + "More")
  })

  it("switches modes without changing source and emits Markdown for rich edits", () => {
    function Draft() {
      const [value, setValue] = useState("Hello")
      return <><MarkdownEditor value={value} onChange={setValue} /><output>{value}</output></>
    }
    render(<Draft />)
    expect(screen.getByRole("textbox")).toHaveAttribute("contenteditable", "true")
    fireEvent.click(screen.getByRole("button", { name: "Source Markdown" }))
    expect(screen.getByRole("textbox")).toHaveValue("Hello")
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "World" } })
    fireEvent.click(screen.getByRole("button", { name: "Rich text" }))
    expect(screen.getByRole("textbox")).toHaveTextContent("World")
    fireEvent.click(screen.getByRole("button", { name: "Heading 2" }))
    expect(screen.getByRole("status")).toHaveTextContent("## World")
  })

  it.each(["1. First", "- First", "- [ ] First"])("keeps rich editing after Enter in %j", (initial) => {
    function Draft() {
      const [value, setValue] = useState(initial)
      return <MarkdownEditor value={value} onChange={setValue} />
    }
    render(<Draft />)
    const textbox = screen.getByRole("textbox")
    const editor = (textbox as HTMLElement & { editor: Editor }).editor
    act(() => { editor.commands.focus("end", { scrollIntoView: false }) })
    fireEvent.keyDown(textbox, { key: "Enter", code: "Enter", keyCode: 13 })
    expect(screen.getByRole("textbox")).toBe(textbox)
    expect(textbox).toHaveAttribute("contenteditable", "true")
    expect(textbox.querySelectorAll("li")).toHaveLength(2)
    expect(textbox.querySelectorAll("li")[1].querySelector("p")?.textContent).toBe("")
    expect(screen.getByRole("button", { name: "Rich text" })).toHaveAttribute("aria-pressed", "true")
    act(() => { editor.commands.insertContent("Second") })
    expect(textbox.querySelectorAll("li")[1]).toHaveTextContent("Second")
    fireEvent.click(screen.getByRole("button", { name: "Source Markdown" }))
    expect(screen.getByRole("textbox")).toHaveValue(initial + "\n" + (initial.startsWith("1.") ? "2. Second" : initial.replace("First", "Second")))
  })

  it("disables both source and rich editing when saving", () => {
    const { rerender } = render(<MarkdownEditor value="Hello" onChange={() => {}} disabled />)
    expect(screen.getByRole("textbox")).toHaveAttribute("contenteditable", "false")
    expect(screen.getByRole("button", { name: "Bold" })).toBeDisabled()
    rerender(<MarkdownEditor value="<!-- source -->" onChange={() => {}} disabled />)
    expect(screen.getByRole("textbox")).toBeDisabled()
  })

  it.each(["", " \n", "Hello\n", "* First\n* Second\n", "1. First\n2. ", "1. ", "1. First\n   1. Nested\n   2. ", "# Heading\n\nParagraph\n"])("allows harmless formatting normalization without changing %j", (value) => {
    const onChange = vi.fn()
    render(<MarkdownEditor value={value} onChange={onChange} />)
    expect(screen.getByRole("textbox")).toHaveAttribute("contenteditable", "true")
    fireEvent.click(screen.getByRole("button", { name: "Source Markdown" }))
    expect(screen.getByRole("textbox")).toHaveValue(value)
    expect(onChange).not.toHaveBeenCalled()
  })

  it("rejects executable link targets before inserting a link", () => {
    const onChange = vi.fn()
    render(<MarkdownEditor value="" onChange={onChange} />)
    fireEvent.click(screen.getByRole("button", { name: "Link" }))
    fireEvent.change(screen.getByRole("textbox", { name: "Link URL" }), { target: { value: "javascript:alert(1)" } })
    fireEvent.click(screen.getByRole("button", { name: "Apply link" }))
    expect(screen.getByRole("alert")).toBeInTheDocument()
    expect(onChange).not.toHaveBeenCalled()
    fireEvent.change(screen.getByRole("textbox", { name: "Link URL" }), { target: { value: "https://example.com" } })
    fireEvent.click(screen.getByRole("button", { name: "Apply link" }))
    expect(onChange).toHaveBeenLastCalledWith("[https://example.com](https://example.com)")
  })
})
