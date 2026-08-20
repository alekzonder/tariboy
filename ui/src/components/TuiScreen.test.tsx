import { act, render, screen, fireEvent, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, it, expect, vi, beforeEach } from "vitest"
import { TuiScreen } from "@/components/TuiScreen"
import type { TerminalSocketController } from "@/hooks/useTerminalSocket"
import * as api from "@/lib/api"
import { openExternalUrl } from "@/lib/desktop"
import { toast } from "sonner"

// xterm.js touches the real DOM (canvas/renderer) and pulls in a stylesheet,
// neither of which jsdom supports. Mock both packages so `Terminal`/`FitAddon`
// are plain spies the component can construct, open, and dispose without a
// browser. The CSS import is stubbed to a no-op module.
vi.mock("@xterm/xterm/css/xterm.css", () => ({}))

// vi.mock is hoisted above imports, so the fakes must be created inside a
// vi.hoisted block (which runs first) — a plain class declaration would be in
// its TDZ when the mock factory runs.
const { termInstances, webLinkAddons, FakeTerminal, FakeFitAddon, FakeWebLinksAddon } =
  vi.hoisted(() => {
    const instances: FakeTerminal[] = []
    const addons: FakeWebLinksAddon[] = []
    class FakeTerminal {
      open = vi.fn()
      onData = vi.fn()
      write = vi.fn()
      dispose = vi.fn()
      loadAddon = vi.fn()
      focus = vi.fn()
      cols = 80
      rows = 24
      constructor() {
        instances.push(this)
      }
    }
    class FakeFitAddon {
      fit = vi.fn()
    }
    // Captures the activation callback the component hands the real addon, so
    // tests can drive a "link click" without xterm's DOM link provider.
    class FakeWebLinksAddon {
      dispose = vi.fn()
      handler: (event: MouseEvent, text: string) => void | Promise<void>
      options?: { urlRegex?: RegExp }
      constructor(
        handler: (event: MouseEvent, text: string) => void | Promise<void>,
        options?: { urlRegex?: RegExp },
      ) {
        this.handler = handler
        this.options = options
        addons.push(this)
      }
    }
    return {
      termInstances: instances,
      webLinkAddons: addons,
      FakeTerminal,
      FakeFitAddon,
      FakeWebLinksAddon,
    }
  })
vi.mock("@xterm/xterm", () => ({ Terminal: FakeTerminal }))
vi.mock("@xterm/addon-fit", () => ({ FitAddon: FakeFitAddon }))
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: FakeWebLinksAddon }))
vi.mock("@/lib/desktop", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/desktop")>()),
  openExternalUrl: vi.fn(async () => null),
}))
vi.mock("@/components/SendFilesButton", () => ({
  SendFilesButton: ({
    onUploaded,
  }: {
    onUploaded: (paths: string[]) => void
  }) => (
    <button type="button" onClick={() => onUploaded(["/remote/private/file.txt"])}>
      Send files
    </button>
  ),
}))

function ctrl(over: Partial<TerminalSocketController> = {}): TerminalSocketController {
  return {
    status: "open",
    absent: false,
    send: vi.fn(),
    sendResize: vi.fn(),
    attachTerm: vi.fn(),
    name: "a1",
    reconnect: vi.fn(),
    ...over,
  }
}

describe("TuiScreen", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    termInstances.length = 0
    webLinkAddons.length = 0
    localStorage.clear()
  })

  it("mounts the terminal container and hands the term to the controller", () => {
    const c = ctrl()
    render(<TuiScreen controller={c} />)
    expect(screen.getByTestId("tui-screen")).toBeInTheDocument()
    expect(c.attachTerm).toHaveBeenCalledTimes(1)
    // The mounted Terminal was opened into the container.
    expect(termInstances[0].open).toHaveBeenCalled()
  })

  it("uses flush pane chrome only for the Workspace surface", () => {
    const standalone = render(<TuiScreen controller={ctrl()} />)
    expect(screen.getByTestId("tui-screen")).toHaveClass("rounded-md", "border")
    standalone.unmount()

    render(<TuiScreen controller={ctrl()} surface="workspace" />)
    expect(screen.getByTestId("tui-screen")).toHaveAttribute(
      "data-terminal-surface",
      "workspace",
    )
    expect(screen.getByTestId("tui-screen")).not.toHaveClass("rounded-md", "border")
    expect(screen.getByTestId("terminal-toolbar")).toHaveClass("border-t")
  })

  it("disposes the terminal attachment when its tile unmounts", () => {
    const view = render(<TuiScreen controller={ctrl()} persistDraft={false} />)
    const term = termInstances[0]

    view.unmount()

    expect(term.dispose).toHaveBeenCalledOnce()
  })

  it("fits and sends the initial size via controller.sendResize on mount", () => {
    const c = ctrl()
    render(<TuiScreen controller={c} />)
    expect(c.sendResize).toHaveBeenCalledTimes(1)
    expect(c.sendResize).toHaveBeenCalledWith(termInstances[0].cols, termInstances[0].rows)
  })

  it("wires keystrokes from the terminal to controller.send", () => {
    const c = ctrl()
    render(<TuiScreen controller={c} />)
    // term.onData((d) => send(d)) is registered during mount; invoke the
    // captured callback to assert it forwards to controller.send.
    const onDataCallback = termInstances[0].onData.mock.calls[0][0] as (d: string) => void
    onDataCallback("hello")
    expect(c.send).toHaveBeenCalledWith("hello")
  })

  it("a hotkey button sends its raw bytes via controller.send", () => {
    const c = ctrl()
    render(<TuiScreen controller={c} />)
    fireEvent.click(screen.getByRole("button", { name: "Ctrl-C" }))
    expect(c.send).toHaveBeenCalledWith("\x03")
  })

  it("re-fits and re-sends the size when the socket opens", async () => {
    // The mount fit can land while the ws is still CONNECTING, so its resize
    // frame is dropped and the fresh `tmux attach` client keeps the size it was
    // dialed at — the shrunken screen seen right after switching agents.
    const c = ctrl({ status: "connecting" })
    const { rerender } = render(<TuiScreen controller={c} />)
    const before = (c.sendResize as ReturnType<typeof vi.fn>).mock.calls.length

    const open = { ...c, status: "open" as const }
    rerender(<TuiScreen controller={open} />)
    await waitFor(() =>
      expect((open.sendResize as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(
        before,
      ),
    )
    expect(open.sendResize).toHaveBeenLastCalledWith(
      termInstances[0].cols,
      termInstances[0].rows,
    )
  })

  it("re-fits on a window resize so the terminal follows the viewport", async () => {
    const c = ctrl()
    render(<TuiScreen controller={c} />)
    const before = (c.sendResize as ReturnType<typeof vi.fn>).mock.calls.length

    fireEvent(window, new Event("resize"))
    await waitFor(() =>
      expect((c.sendResize as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(before),
    )
  })

  it("shows the Start panel when the session is absent and wires onStart", () => {
    const onStart = vi.fn()
    render(<TuiScreen controller={ctrl({ absent: true })} onStart={onStart} />)
    expect(screen.getByText(/not running or not interactive/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole("button", { name: "Start" }))
    expect(onStart).toHaveBeenCalled()
  })

  it("the compose button opens the modal and Send routes the draft through controller.send", async () => {
    const c = ctrl()
    render(<TuiScreen controller={c} />)
    fireEvent.click(screen.getByRole("button", { name: /compose/i }))
    const textarea = await screen.findByLabelText("Text to inject")
    fireEvent.change(textarea, { target: { value: "ls -la" } })
    fireEvent.click(screen.getByRole("button", { name: "Send" }))
    expect(c.send).toHaveBeenCalledWith("ls -la")
    await waitFor(() =>
      expect(screen.queryByLabelText("Text to inject")).not.toBeInTheDocument(),
    )
  })

  it("keeps compose text in memory when persistent drafts are disabled", async () => {
    const c = ctrl({ name: "same-name-on-another-host" })
    render(<TuiScreen controller={c} persistDraft={false} />)

    fireEvent.click(screen.getByRole("button", { name: /compose/i }))
    fireEvent.change(await screen.findByLabelText("Text to inject"), {
      target: { value: "private operator text /remote/workdir/file.txt" },
    })

    expect(localStorage.length).toBe(0)
  })

  it("keeps uploaded server paths out of storage when persistent drafts are disabled", async () => {
    render(<TuiScreen controller={ctrl()} persistDraft={false} />)

    fireEvent.click(screen.getByRole("button", { name: "Send files" }))

    expect(await screen.findByLabelText("Text to inject")).toHaveValue(
      "/remote/private/file.txt",
    )
    expect(localStorage.length).toBe(0)
  })

  it("case 1: drops one file on the live terminal, uploads it, appends its path, and opens compose", async () => {
    const upload = vi.spyOn(api, "agentUploadFile").mockResolvedValue({
      path: "one.txt", abs: "/cwd/.tariboy/files/one.txt", bytes: 1,
    })
    render(<TuiScreen controller={ctrl()} />)

    fireEvent.drop(screen.getByTestId("tui-screen"), {
      dataTransfer: { files: [new File(["one"], "one.txt")] },
    })

    await waitFor(() => expect(upload).toHaveBeenCalledWith("a1", expect.any(File), undefined))
    expect(await screen.findByLabelText("Text to inject")).toHaveValue("/cwd/.tariboy/files/one.txt")
  })

  it("case 2: drops multiple files on the live terminal and uploads every file", async () => {
    const upload = vi.spyOn(api, "agentUploadFile")
      .mockResolvedValueOnce({ path: "one.txt", abs: "/cwd/one.txt", bytes: 1 })
      .mockResolvedValueOnce({ path: "two.txt", abs: "/cwd/two.txt", bytes: 1 })
    render(<TuiScreen controller={ctrl()} />)

    fireEvent.drop(screen.getByTestId("tui-screen"), {
      dataTransfer: { files: [new File(["one"], "one.txt"), new File(["two"], "two.txt")] },
    })

    await waitFor(() => expect(upload).toHaveBeenCalledTimes(2))
    expect(await screen.findByLabelText("Text to inject")).toHaveValue("/cwd/one.txt\n/cwd/two.txt")
  })

  it("case 3: prevents the browser default on file dragover and drop", () => {
    render(<TuiScreen controller={ctrl()} />)
    const target = screen.getByTestId("tui-screen")
    // Protected mode: during dragover a real browser exposes no files, only
    // `types`. The drop half keeps its files, which a real drop does expose.
    const dragover = fireEvent.dragOver(target, { dataTransfer: { files: [], types: ["Files"] } })
    const drop = fireEvent.drop(target, { dataTransfer: { files: [new File(["x"], "x.txt")] } })

    expect(dragover).toBe(false)
    expect(drop).toBe(false)
  })

  it("case 4: ignores a text-only drag without uploading", async () => {
    const upload = vi.spyOn(api, "agentUploadFile")
    render(<TuiScreen controller={ctrl()} />)
    const target = screen.getByTestId("tui-screen")

    fireEvent.drop(target, {
      dataTransfer: { files: [], items: [{ kind: "string", type: "text/plain" }] },
    })

    await Promise.resolve()
    expect(upload).not.toHaveBeenCalled()
    expect(target).toHaveAttribute("data-file-drag-active", "false")
  })

  it("case 5: drops on the absent panel, uploads paths, and toasts them without opening compose", async () => {
    const upload = vi.spyOn(api, "agentUploadFile").mockResolvedValue({
      path: "staged.txt", abs: "/cwd/staged.txt", bytes: 1,
    })
    const success = vi.spyOn(toast, "success")
    render(<TuiScreen controller={ctrl({ absent: true })} />)

    fireEvent.drop(screen.getByTestId("tui-absent-drop-target"), {
      dataTransfer: { files: [new File(["staged"], "staged.txt")] },
    })

    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
    expect(success).toHaveBeenCalledWith("uploaded: /cwd/staged.txt")
    expect(screen.queryByLabelText("Text to inject")).not.toBeInTheDocument()
  })

  it("case 7: shows a file-drop affordance and clears it after dragleave and drop", async () => {
    const upload = vi.spyOn(api, "agentUploadFile").mockResolvedValue({
      path: "x.txt", abs: "/cwd/x.txt", bytes: 1,
    })
    render(<TuiScreen controller={ctrl()} />)
    const target = screen.getByTestId("tui-screen")

    fireEvent.dragOver(target, { dataTransfer: { files: [], types: ["Files"] } })
    expect(target).toHaveAttribute("data-file-drag-active", "true")
    fireEvent.dragLeave(target)
    expect(target).toHaveAttribute("data-file-drag-active", "false")
    fireEvent.dragOver(target, { dataTransfer: { files: [], types: ["Files"] } })
    fireEvent.drop(target, { dataTransfer: { files: [new File(["x"], "x.txt")] } })
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
    expect(target).toHaveAttribute("data-file-drag-active", "false")
  })

  it("case 8: clears the drop affordance and toasts when an upload fails", async () => {
    vi.spyOn(api, "agentUploadFile").mockRejectedValue(new Error("upload failed"))
    const error = vi.spyOn(toast, "error")
    render(<TuiScreen controller={ctrl()} />)
    const target = screen.getByTestId("tui-screen")

    fireEvent.dragOver(target, { dataTransfer: { files: [], types: ["Files"] } })
    fireEvent.drop(target, { dataTransfer: { files: [new File(["x"], "x.txt")] } })

    await waitFor(() => expect(error).toHaveBeenCalledWith("Error: upload failed"))
    expect(target).toHaveAttribute("data-file-drag-active", "false")
  })

  // A dropped directory: `kind` is "file" and it appears in `files`, but its
  // entry reports `isDirectory`. Only the drop event may read files/entries.
  const dirItem = () => ({ kind: "file", webkitGetAsEntry: () => ({ isDirectory: true }) })
  const fileItem = () => ({ kind: "file", webkitGetAsEntry: () => ({ isDirectory: false }) })

  it("case 9: prevents the default on a dragover that carries files but exposes none (protected mode)", () => {
    render(<TuiScreen controller={ctrl()} />)
    const target = screen.getByTestId("tui-screen")

    const dragover = fireEvent.dragOver(target, { dataTransfer: { files: [], types: ["Files"] } })

    expect(dragover).toBe(false)
  })

  it("case 10: leaves a dragover without a file marker alone", () => {
    render(<TuiScreen controller={ctrl()} />)
    const target = screen.getByTestId("tui-screen")

    const dragover = fireEvent.dragOver(target, {
      dataTransfer: { files: [], types: ["text/plain"] },
    })

    expect(dragover).toBe(true)
    expect(target).toHaveAttribute("data-file-drag-active", "false")
  })

  it("case 11: refuses a drop of only directories with a single toast and no upload", async () => {
    // Resolve rather than reject, so the only toast this test can observe is
    // the folder refusal — never an upload failure leaking in from elsewhere.
    const upload = vi.spyOn(api, "agentUploadFile").mockResolvedValue({
      path: "folder", abs: "/cwd/folder", bytes: 0,
    })
    const error = vi.spyOn(toast, "error")
    render(<TuiScreen controller={ctrl()} />)

    fireEvent.drop(screen.getByTestId("tui-screen"), {
      dataTransfer: { files: [new File([], "folder")], items: [dirItem()] },
    })

    await waitFor(() => expect(error).toHaveBeenCalledTimes(1))
    expect(error).toHaveBeenCalledWith("folders cannot be sent — drop files instead")
    expect(upload).not.toHaveBeenCalled()
    expect(screen.queryByLabelText("Text to inject")).not.toBeInTheDocument()
  })

  it("case 12: uploads only the file when a drop mixes a file and a directory", async () => {
    const upload = vi.spyOn(api, "agentUploadFile").mockResolvedValue({
      path: "one.txt", abs: "/cwd/one.txt", bytes: 1,
    })
    const error = vi.spyOn(toast, "error")
    render(<TuiScreen controller={ctrl()} />)

    fireEvent.drop(screen.getByTestId("tui-screen"), {
      dataTransfer: {
        files: [new File(["one"], "one.txt"), new File([], "folder")],
        items: [fileItem(), dirItem()],
      },
    })

    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
    expect(upload.mock.calls[0][1]).toHaveProperty("name", "one.txt")
    expect(error).toHaveBeenCalledWith("folders cannot be sent — drop files instead")
  })

  it("case 13: still uploads an ordinary file dropped through the items path", async () => {
    const upload = vi.spyOn(api, "agentUploadFile").mockResolvedValue({
      path: "one.txt", abs: "/cwd/one.txt", bytes: 1,
    })
    const error = vi.spyOn(toast, "error")
    render(<TuiScreen controller={ctrl()} />)

    fireEvent.drop(screen.getByTestId("tui-screen"), {
      dataTransfer: { files: [new File(["one"], "one.txt")], items: [fileItem()] },
    })

    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
    expect(await screen.findByLabelText("Text to inject")).toHaveValue("/cwd/one.txt")
    expect(error).not.toHaveBeenCalled()
  })

  // A drag that advertises "Files" but yields nothing on drop — promised or
  // virtual files, a mail attachment, a remote mount, a source that cancels
  // late. Our dragover has already claimed the drag, so the drop fires and the
  // browser would navigate away to the dropped item unless we prevent it.
  it("case 14: prevents the browser default on a drop that carries no file at all", () => {
    render(<TuiScreen controller={ctrl()} />)
    const target = screen.getByTestId("tui-screen")

    fireEvent.dragOver(target, { dataTransfer: { files: [], types: ["Files"] } })
    const drop = fireEvent.drop(target, { dataTransfer: { files: [], types: ["Files"] } })

    expect(drop).toBe(false)
  })

  it("case 15: clears the drop affordance after a drop that carries no file at all", () => {
    render(<TuiScreen controller={ctrl()} />)
    const target = screen.getByTestId("tui-screen")

    fireEvent.dragOver(target, { dataTransfer: { files: [], types: ["Files"] } })
    fireEvent.drop(target, { dataTransfer: { files: [], types: ["Files"] } })

    expect(target).toHaveAttribute("data-file-drag-active", "false")
  })

  // Tmux scrollback (SUPER-103). The client has no authoritative tmux mode
  // event, so these assert the *requested* state the toolbar shows, and the
  // exact raw bytes that go over the existing socket — never a resize frame.
  describe("scrollback", () => {
    const enterScrollback = () =>
      fireEvent.click(screen.getByRole("button", { name: "Scrollback" }))
    const SCROLLBACK_CONTROLS = [
      "Scrollback",
      "Page back",
      "Page forward",
      "Exit scrollback",
    ]

    it("scrollback case 1a: the Scrollback control sends exactly the tmux copy-mode bytes", () => {
      const c = ctrl()
      render(<TuiScreen controller={c} />)

      enterScrollback()

      expect(c.send).toHaveBeenCalledWith("\x02[")
    })

    it("scrollback case 1b: entering scrollback sends no resize frame", () => {
      const c = ctrl()
      render(<TuiScreen controller={c} />)
      const before = (c.sendResize as ReturnType<typeof vi.fn>).mock.calls.length

      enterScrollback()

      expect((c.sendResize as ReturnType<typeof vi.fn>).mock.calls.length).toBe(before)
    })

    it("scrollback case 2a: shows the requested-state status while viewing scrollback", () => {
      render(<TuiScreen controller={ctrl()} />)

      enterScrollback()

      expect(screen.getByText("Viewing scrollback")).toBeInTheDocument()
    })

    it("scrollback case 2b: page back sends the PageUp sequence", () => {
      const c = ctrl()
      render(<TuiScreen controller={c} />)
      enterScrollback()

      fireEvent.click(screen.getByRole("button", { name: "Page back" }))

      expect(c.send).toHaveBeenLastCalledWith("\x1b[5~")
    })

    it("scrollback case 2c: page forward sends the PageDown sequence", () => {
      const c = ctrl()
      render(<TuiScreen controller={c} />)
      enterScrollback()

      fireEvent.click(screen.getByRole("button", { name: "Page forward" }))

      expect(c.send).toHaveBeenLastCalledWith("\x1b[6~")
    })

    it("scrollback case 3a: Exit scrollback sends the tmux copy-mode quit key", () => {
      const c = ctrl()
      render(<TuiScreen controller={c} />)
      enterScrollback()

      fireEvent.click(screen.getByRole("button", { name: "Exit scrollback" }))

      expect(c.send).toHaveBeenLastCalledWith("q")
    })

    it("scrollback case 3b: Exit scrollback clears the visible status", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      fireEvent.click(screen.getByRole("button", { name: "Exit scrollback" }))

      expect(screen.queryByText("Viewing scrollback")).not.toBeInTheDocument()
    })

    it("scrollback case 4a: every scrollback control has an accessible name", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      expect(
        ["Scrollback", "Page back", "Page forward", "Exit scrollback"].map(
          (name) => screen.getByRole("button", { name }).tagName,
        ),
      ).toEqual(["BUTTON", "BUTTON", "BUTTON", "BUTTON"])
    })

    it("scrollback case 4b: the Scrollback control is reachable and activatable by keyboard", async () => {
      const user = userEvent.setup()
      const c = ctrl()
      render(<TuiScreen controller={c} />)

      screen.getByRole("button", { name: "Scrollback" }).focus()
      await user.keyboard("{Enter}")

      expect(c.send).toHaveBeenCalledWith("\x02[")
    })

    it("scrollback case 4c: explains how to leave scrollback and that live input is paused", () => {
      render(<TuiScreen controller={ctrl()} />)

      enterScrollback()

      expect(
        screen.getByText(/live input is paused.*press q in the terminal or use Exit scrollback/i),
      ).toBeInTheDocument()
    })

    it("scrollback case 5a: the requested state resets when the session goes absent and returns", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      enterScrollback()

      rerender(<TuiScreen controller={{ ...c, absent: true }} />)
      rerender(<TuiScreen controller={{ ...c, absent: false }} />)

      expect(screen.queryByText("Viewing scrollback")).not.toBeInTheDocument()
    })

    it("scrollback case 5b: the requested state resets when the terminal identity changes", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      enterScrollback()

      rerender(<TuiScreen controller={{ ...c, name: "a2" }} />)

      expect(screen.queryByText("Viewing scrollback")).not.toBeInTheDocument()
    })

    it("scrollback case 5c: a remounted terminal starts in live mode", () => {
      const c = ctrl()
      const view = render(<TuiScreen controller={c} />)
      enterScrollback()
      view.unmount()

      render(<TuiScreen controller={c} />)

      expect(screen.queryByText("Viewing scrollback")).not.toBeInTheDocument()
    })

    // A disabled control voids any negative test that drives it: React never
    // dispatches the handler, so "nothing was sent" would hold even with the
    // guard deleted. These three each pin one reachable property instead —
    // the disabling itself, the transition that removes the controls, and the
    // boundary showing what the closed state deliberately does NOT stop.
    it("scrollback case 5d1: the Scrollback control is disabled once the socket is closed", () => {
      render(<TuiScreen controller={ctrl({ status: "closed" })} />)

      expect(screen.getByRole("button", { name: "Scrollback" })).toBeDisabled()
    })

    it("scrollback case 5d2: closing the socket removes every copy-mode control and sends nothing further", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      enterScrollback()
      // Sanity: mounting straight at "closed" would leave these absent from the
      // initial state alone, which proves nothing about the close.
      expect(copyModeControls()).not.toContain(null)
      // Entering already recorded the "\x02[" write, so a bare
      // not.toHaveBeenCalled would assert about that call, not about the close.
      vi.mocked(c.send).mockClear()

      rerender(<TuiScreen controller={{ ...c, status: "closed" }} />)

      expect(copyModeControls()).toEqual([null, null, null])
      expect(c.send).not.toHaveBeenCalled()
    })

    it("scrollback case 5e: the closed-socket guard covers only the copy-mode controls - a plain hotkey button still sends", () => {
      // Correct, not a bug: the plain hotkeys are deliberately left unguarded
      // in the component, and a write to a dead socket is stopped one layer
      // down by the readyState check in useTerminalSocket's send. Pinning the
      // boundary here stops a later reader from "fixing" the asymmetry by
      // routing every send through the copy-mode guard without a decision.
      const c = ctrl({ status: "closed" })
      render(<TuiScreen controller={c} />)

      fireEvent.click(screen.getByRole("button", { name: "Esc" }))

      expect(c.send).toHaveBeenCalledWith("\x1b")
    })

    // The documented primary exit is `q` typed in the focused terminal, and the
    // session closing ends copy-mode outright. Both must drop the requested
    // state, or the toolbar keeps painting a scrollback banner over live output.
    const typeInTerminal = (d: string) => {
      const onData = termInstances[0].onData.mock.calls[0][0] as (d: string) => void
      act(() => onData(d))
    }
    const copyModeControls = () =>
      SCROLLBACK_CONTROLS.slice(1).map((name) => screen.queryByRole("button", { name }))

    it("scrollback case a: typing q in the terminal leaves scrollback", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      typeInTerminal("q")

      expect(screen.queryByText("Viewing scrollback")).not.toBeInTheDocument()
      expect(copyModeControls()).toEqual([null, null, null])
    })

    it("scrollback case b: typing q still forwards the byte to controller.send", () => {
      const c = ctrl()
      render(<TuiScreen controller={c} />)
      enterScrollback()

      typeInTerminal("q")

      expect(c.send).toHaveBeenLastCalledWith("q")
    })

    it("scrollback case c: a multi-character chunk containing q does not leave scrollback", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      typeInTerminal("quit")

      expect(screen.getByText("Viewing scrollback")).toBeInTheDocument()
      expect(copyModeControls()).not.toContain(null)
    })

    it("scrollback case d: typing q outside scrollback changes nothing and still forwards", () => {
      const c = ctrl()
      render(<TuiScreen controller={c} />)

      typeInTerminal("q")

      expect(screen.queryByText("Viewing scrollback")).not.toBeInTheDocument()
      expect(copyModeControls()).toEqual([null, null, null])
      expect(c.send).toHaveBeenLastCalledWith("q")
    })

    it("scrollback case e: a closed socket clears the requested state", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      enterScrollback()

      rerender(<TuiScreen controller={{ ...c, status: "closed" }} />)

      expect(screen.queryByText("Viewing scrollback")).not.toBeInTheDocument()
      expect(copyModeControls()).toEqual([null, null, null])
    })

    it("scrollback case g: typing Escape does not leave scrollback", () => {
      // Deliberate: Escape cancels copy-mode under tmux's default emacs table
      // but is clear-selection under copy-mode-vi, and the browser cannot know
      // which table the session got. Inferring an exit from it would paint a
      // live toolbar while tmux still swallows every keystroke.
      const c = ctrl()
      render(<TuiScreen controller={c} />)
      enterScrollback()

      typeInTerminal("\x1b")

      expect(screen.getByText("Viewing scrollback")).toBeInTheDocument()
      expect(copyModeControls()).not.toContain(null)
      expect(c.send).toHaveBeenLastCalledWith("\x1b")
    })

    // Leaving scrollback used to be silent to assistive technology and dropped
    // keyboard focus, because the live region and the focused Exit button both
    // lived inside the conditional fragment: a live region that unmounts
    // announces nothing, and the focus fell back to <body>. These pin the
    // persistent region, the *earned* announcement (never on mount, never on a
    // `q` typed outside scrollback, never when the session merely closed) and
    // the one path that restores focus. The case letters carry a 192 prefix so
    // the mutation reports cannot confuse them with cases a-g above.
    const liveRegion = () => screen.getByRole("status")
    const scrollbackButton = () => screen.getByRole("button", { name: "Scrollback" })
    const EXIT_ANNOUNCEMENT = "Returned to live output"
    const exitScrollback = () =>
      fireEvent.click(screen.getByRole("button", { name: "Exit scrollback" }))

    it("scrollback case 192a: the live region stays in the DOM outside scrollback", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      exitScrollback()

      expect(liveRegion()).toBeInTheDocument()
    })

    it("scrollback case 192b: entering scrollback puts the banner and hint inside the region", () => {
      render(<TuiScreen controller={ctrl()} />)

      enterScrollback()

      expect(within(liveRegion()).getByText("Viewing scrollback")).toBeInTheDocument()
      expect(
        within(liveRegion()).getByText(
          /live input is paused.*press q in the terminal or use Exit scrollback/i,
        ),
      ).toBeInTheDocument()
    })

    it("scrollback case 192c: Exit scrollback announces the return to live output", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      exitScrollback()

      const announced = liveRegion().textContent ?? ""
      expect(announced).toContain(EXIT_ANNOUNCEMENT)
      // The desktop spec asserts the whole toolbar's text contains neither of
      // these once scrollback is left, so the announcement may not carry them.
      expect(announced).not.toContain("Viewing scrollback")
      expect(announced).not.toContain("Exit scrollback")
    })

    it("scrollback case 192d: Exit scrollback returns focus to the Scrollback control", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      exitScrollback()

      expect(document.activeElement).toBe(scrollbackButton())
    })

    it("scrollback case 192e: leaving with q leaves focus in the terminal", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      typeInTerminal("q")

      expect(document.activeElement).not.toBe(scrollbackButton())
    })

    it("scrollback case 192f: a closed socket neither throws nor steals focus", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      enterScrollback()

      expect(() =>
        rerender(<TuiScreen controller={{ ...c, status: "closed" }} />),
      ).not.toThrow()

      expect(document.activeElement).not.toBe(scrollbackButton())
    })

    it("scrollback case 192g: the region is empty on first render", () => {
      render(<TuiScreen controller={ctrl()} />)

      expect(liveRegion()).toBeInTheDocument()
      expect(liveRegion().textContent).toBe("")
    })

    it("scrollback case 192h: leaving with q announces the return to live output", () => {
      render(<TuiScreen controller={ctrl()} />)
      enterScrollback()

      typeInTerminal("q")

      expect(liveRegion().textContent).toContain(EXIT_ANNOUNCEMENT)
    })

    it("scrollback case 192i: a closed socket announces nothing", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      enterScrollback()

      rerender(<TuiScreen controller={{ ...c, status: "closed" }} />)

      expect(liveRegion().textContent).toBe("")
    })

    it("scrollback case 192j: a replacement terminal does not inherit the announcement", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      enterScrollback()
      exitScrollback()

      rerender(<TuiScreen controller={{ ...c, name: "a2" }} />)

      expect(liveRegion().textContent).toBe("")
    })

    it("scrollback case 192k: typing q outside scrollback announces nothing and still forwards", () => {
      const c = ctrl()
      render(<TuiScreen controller={c} />)

      typeInTerminal("q")

      expect(liveRegion().textContent).toBe("")
      expect(c.send).toHaveBeenLastCalledWith("q")
    })

    it("scrollback case 192l: typing q after the socket closed announces nothing", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      enterScrollback()
      rerender(<TuiScreen controller={{ ...c, status: "closed" }} />)

      typeInTerminal("q")

      expect(liveRegion().textContent).toBe("")
    })

    it("scrollback case 7: the same controls come from the shared component on Console and Workspace", () => {
      const console_ = render(<TuiScreen controller={ctrl()} />)
      enterScrollback()
      const onConsole = screen
        .getAllByRole("button")
        .map((b) => b.textContent)
        .filter((t) => /scrollback|Page/i.test(t ?? ""))
      console_.unmount()

      render(<TuiScreen controller={ctrl()} surface="workspace" persistDraft={false} />)
      enterScrollback()
      const onWorkspace = screen
        .getAllByRole("button")
        .map((b) => b.textContent)
        .filter((t) => /scrollback|Page/i.test(t ?? ""))

      expect([onConsole, onWorkspace]).toEqual([SCROLLBACK_CONTROLS, SCROLLBACK_CONTROLS])
    })
  })

  // Cmd+Click web links (SUPER-124). The addon's activation callback is driven
  // directly: xterm's link provider needs a real renderer, and what matters
  // here is the decision the component makes for a given (event, text) pair.
  describe("web links", () => {
    const opened = () => vi.mocked(openExternalUrl)

    /** A MouseEvent stub whose prohibited side-effect APIs are observable. */
    const mouse = (over: Partial<Record<"metaKey" | "ctrlKey" | "altKey" | "shiftKey", boolean>> = {}) => {
      const e = {
        metaKey: false,
        ctrlKey: false,
        altKey: false,
        shiftKey: false,
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
      }
      return Object.assign(e, over) as unknown as MouseEvent & {
        preventDefault: ReturnType<typeof vi.fn>
        stopPropagation: ReturnType<typeof vi.fn>
      }
    }

    /** Mounts a live terminal and returns its single captured activation callback. */
    const mountLink = (props: Partial<Parameters<typeof TuiScreen>[0]> = {}) => {
      const view = render(<TuiScreen controller={ctrl()} {...props} />)
      expect(webLinkAddons).toHaveLength(1)
      return { view, activate: webLinkAddons[0].handler, addon: webLinkAddons[0] }
    }

    // Everything the native boundary (SUPER-123) rejects, plus text that is not
    // an absolute URL at all. None of it may reach the opener.
    const REJECTED = [
      "file:///tmp/a",
      "javascript:alert(1)",
      "data:text/plain,x",
      "custom:value",
      "//example.test",
      "/tmp/a",
      "./a",
      "example.test",
      "%",
      "http://",
      "",
    ]

    it("web links case 1: loads exactly one web-links addon into the mounted terminal", () => {
      const { addon } = mountLink()
      expect(termInstances[0].loadAddon).toHaveBeenCalledWith(addon)
    })

    it("web links case 2: a Command-click on an https link opens it externally", async () => {
      const { activate } = mountLink()
      await activate(mouse({ metaKey: true }), "https://example.test/a")
      expect(opened()).toHaveBeenCalledExactlyOnceWith("https://example.test/a")
    })

    it("web links case 3: a Command-click on an http link opens it externally", async () => {
      const { activate } = mountLink()
      await activate(mouse({ metaKey: true }), "http://example.test/a")
      expect(opened()).toHaveBeenCalledExactlyOnceWith("http://example.test/a")
    })

    it("web links case 4: a Command-click preserves the link's query string", async () => {
      const { activate } = mountLink()
      await activate(mouse({ metaKey: true }), "https://example.test/a?token=secret")
      expect(opened()).toHaveBeenCalledExactlyOnceWith("https://example.test/a?token=secret")
    })

    // One test per modifier state: a failing expect() aborts its own block, so
    // bundling these would let the first failure hide the other three.
    for (const [label, over] of [
      ["a plain click", {}],
      ["a Ctrl-click", { ctrlKey: true }],
      ["an Alt-click", { altKey: true }],
      ["a Shift-click", { shiftKey: true }],
    ] as const) {
      it(`web links case 5 (${label}): does not open, and touches no event or focus API`, async () => {
        const { activate } = mountLink()
        const event = mouse(over)

        await activate(event, "https://example.test/a")

        expect(opened()).not.toHaveBeenCalled()
        expect(event.preventDefault).not.toHaveBeenCalled()
        expect(event.stopPropagation).not.toHaveBeenCalled()
        expect(termInstances[0].focus).not.toHaveBeenCalled()
      })
    }

    // One test per rejected input, so the run names every offender rather than
    // stopping at the first one.
    for (const text of REJECTED) {
      it(`web links case 6 (${JSON.stringify(text)}): is a no-op even with Command held`, async () => {
        const { activate } = mountLink()
        const event = mouse({ metaKey: true })

        await activate(event, text)

        expect(opened()).not.toHaveBeenCalled()
        expect(event.preventDefault).not.toHaveBeenCalled()
        expect(event.stopPropagation).not.toHaveBeenCalled()
        expect(termInstances[0].focus).not.toHaveBeenCalled()
      })
    }

    it("web links case 7: a rejected open surfaces a fixed message that leaks no URL", async () => {
      const error = vi.spyOn(toast, "error")
      opened().mockRejectedValueOnce(new Error("cannot open external web link"))
      const { activate } = mountLink()

      await activate(mouse({ metaKey: true }), "https://example.test/a?token=secret")

      expect(error).toHaveBeenCalledExactlyOnceWith("Could not open web link")
      expect(JSON.stringify(error.mock.calls)).not.toMatch(/example\.test|token|secret/)
    })

    it("web links case 8: a synchronous throw is absorbed the same way", async () => {
      const error = vi.spyOn(toast, "error")
      opened().mockImplementationOnce(() => {
        throw new Error("boom https://example.test/a?token=secret")
      })
      const { activate } = mountLink()

      await expect(
        activate(mouse({ metaKey: true }), "https://example.test/a?token=secret"),
      ).resolves.not.toThrow()
      expect(error).toHaveBeenCalledExactlyOnceWith("Could not open web link")
      expect(JSON.stringify(error.mock.calls)).not.toMatch(/example\.test|token|secret/)
    })

    it("web links case 9: unmount disposes the addon before the terminal", () => {
      const { view, addon } = mountLink()

      view.unmount()

      expect(addon.dispose).toHaveBeenCalledOnce()
      expect(addon.dispose.mock.invocationCallOrder[0]).toBeLessThan(
        termInstances[0].dispose.mock.invocationCallOrder[0],
      )
    })

    it("web links case 10: an agent change disposes the old addon and loads exactly one new one", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      const first = webLinkAddons[0]

      rerender(<TuiScreen controller={{ ...c, name: "a2" }} />)

      expect(first.dispose).toHaveBeenCalledOnce()
      expect(webLinkAddons).toHaveLength(2)
    })

    it("web links case 11: a live-to-absent transition disposes the addon", () => {
      const c = ctrl()
      const { rerender } = render(<TuiScreen controller={c} />)
      const first = webLinkAddons[0]

      rerender(<TuiScreen controller={{ ...c, absent: true }} />)

      expect(first.dispose).toHaveBeenCalledOnce()
      expect(webLinkAddons).toHaveLength(1)
    })

    it("web links case 12: the Workspace surface behaves identically to standalone", async () => {
      const { activate } = mountLink({ surface: "workspace", persistDraft: false })
      await activate(mouse({ metaKey: true }), "https://example.test/a")
      expect(opened()).toHaveBeenCalledExactlyOnceWith("https://example.test/a")

      opened().mockClear()
      await activate(mouse(), "https://example.test/a")
      expect(opened()).not.toHaveBeenCalled()
    })
  })
})
