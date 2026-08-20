import { HelpCircle, Keyboard } from "lucide-react"
import {
  useEffect,
  useRef,
  useState,
  type ClipboardEvent as ReactClipboardEvent,
} from "react"
import { toast } from "sonner"
import { FitAddon } from "@xterm/addon-fit"
import { WebLinksAddon } from "@xterm/addon-web-links"
import { Terminal } from "@xterm/xterm"
import "@xterm/xterm/css/xterm.css"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Textarea } from "@/components/ui/textarea"
import { SendFilesButton } from "@/components/SendFilesButton"
import { useFileDropTarget } from "@/hooks/useFileDropTarget"
import { useSendFiles } from "@/hooks/useSendFiles"
import type { TerminalSocketController } from "@/hooks/useTerminalSocket"
import type { Daemon } from "@/lib/daemons"
import { openExternalUrl } from "@/lib/desktop"
import { HOTKEY_BYTES } from "@/lib/terminalBytes"
import { buildPathsText, useTuiDraft } from "@/lib/tui"
import { cn } from "@/lib/utils"

// Tailwind's default `font-mono` stack — mirrored here so the xterm canvas
// renders in the same monospace face as the rest of the app.
const MONO_FONT =
  'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace'

// Zinc dark palette matching the old snapshot terminal (bg zinc-950, fg zinc-100).
const TERM_THEME = {
  background: "#09090b",
  foreground: "#e4e4e7",
  cursor: "#e4e4e7",
  cursorAccent: "#09090b",
  selectionBackground: "#3f3f46",
}

// One-click hotkeys, each a raw byte sequence written straight to the socket.
const HOTKEYS: { label: string; bytes: string }[] = [
  { label: "Esc", bytes: HOTKEY_BYTES.esc },
  { label: "Enter", bytes: HOTKEY_BYTES.enter },
  { label: "Ctrl-C", bytes: HOTKEY_BYTES.ctrlc },
  { label: "↑", bytes: HOTKEY_BYTES.up },
  { label: "↓", bytes: HOTKEY_BYTES.down },
]

// Tmux copy-mode ("scrollback") key sequences, written raw to the same socket
// as the hotkeys. `\x02[` is tmux's default prefix C-b followed by `[`, which
// enters copy-mode; `q` is copy-mode's quit key. Page back/forward are the
// ordinary terminal PageUp/PageDown sequences, which tmux interprets as
// half/full page movement while in copy-mode.
const COPY_MODE_BYTES = {
  enter: "\x02[",
  exit: "q",
  pageBack: "\x1b[5~",
  pageForward: "\x1b[6~",
} as const

const SCROLLBACK_HINT =
  "Live input is paused while you browse history — press q in the terminal or " +
  "use Exit scrollback to return to live output."

// Announced when the operator actually returns to live output. It renders
// inside the toolbar, and ui/tests/desktop/tmux-scrollback.pw.ts asserts the
// whole toolbar's text contains neither "Viewing scrollback" nor "Exit
// scrollback" once scrollback is left — so this wording must keep containing
// neither substring. That spec is the only independent check on it.
const SCROLLBACK_EXIT_ANNOUNCEMENT = "Returned to live output"

// Failure text for an external open. Fixed on purpose: the terminal line — and
// therefore any token in its query string — must never reach a toast or a log.
const WEB_LINK_ERROR = "Could not open web link"

/** Open a Command-clicked terminal link in the operator's browser.
 *
 * Every other click is left entirely to xterm: no `preventDefault`,
 * `stopPropagation`, or focus call, so selection, mouse reporting and focus
 * behave exactly as they did before. The URL is parsed and its scheme checked
 * here, and again natively in `open_external_url` — this side is a usability
 * filter, not the authorization boundary. */
async function openWebLink(event: MouseEvent, text: string) {
  if (!event.metaKey) return
  let url: URL
  try {
    url = new URL(text)
  } catch {
    // Not an absolute URL (bare host, path, protocol-relative, malformed).
    return
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") return
  try {
    await openExternalUrl(url.toString())
  } catch {
    toast.error(WEB_LINK_ERROR)
  }
}

const HELP_TEXT =
  "This is a live terminal. Click it to focus — keystrokes go straight to the " +
  "session in real time. Use the hotkey buttons for Esc/Enter/Ctrl-C/arrows, or " +
  "the Compose button to draft a multi-line message and inject it (no trailing " +
  "Enter). Double-click selects a word natively; drag to select and copy. " +
  "Scrollback opens tmux's pane history; leave it with q or Exit scrollback."

/** The interactive terminal screen. Mounts a live xterm.js terminal driven by
 * the streaming websocket {@link TerminalSocketController}: PTY bytes are
 * written into the terminal and keystrokes flow back over `controller.send`.
 * A toolbar offers hotkeys, a compose modal (optionally persisted per agent),
 * and a "Send files" button that injects uploaded paths into the stream. When
 * the daemon reports no interactive session, a Start panel is shown instead. */
export function TuiScreen({
  controller,
  onStart,
  fill = false,
  daemon,
  persistDraft = true,
  surface = "standalone",
}: {
  controller: TerminalSocketController
  onStart?: () => void
  /** Fill the parent's height (flex column) instead of a fixed 32rem screen. */
  fill?: boolean
  /** Target host for uploads (undefined = active daemon, null = same-origin),
   * for cross-host views like /terminals. */
  daemon?: Daemon | null
  /** Retain compose text in localStorage. Workspace tiles disable this so
   * operator text and uploaded paths never enter persisted workspace state. */
  persistDraft?: boolean
  /** Standalone Console chrome or a flush surface embedded in a Workspace pane. */
  surface?: "standalone" | "workspace"
}) {
  const { absent, name, send, status } = controller
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [focused, setFocused] = useState(false)
  // Set by the mount effect below; lets other effects (re)fit the live terminal
  // without owning its lifetime. Null while no terminal is mounted.
  const refitRef = useRef<(() => void) | null>(null)

  // Compose modal: Agent mode persists its per-agent draft, while Workspace
  // keeps the draft in component memory only. On Send the text is written to
  // the stream as-is.
  const [modalOpen, setModalOpen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  // We asked tmux to enter copy-mode; there is no authoritative tmux mode event
  // on this socket, so this tracks the *requested* state only and every label
  // reads as a request ("Viewing scrollback"), never as verified tmux state.
  // Cleared on Exit scrollback, on `q` typed in the terminal, when the socket
  // closes, when the terminal identity changes, when the session goes absent,
  // and on unmount. Those are not every way out of copy-mode: leaving it any
  // other way still strands the banner — notably Escape, which cancels under
  // tmux's default emacs table (including via the toolbar's own Esc hotkey) but
  // only clears the selection under copy-mode-vi, so we cannot infer it.
  const [copyModeRequested, setCopyModeRequested] = useState(false)
  // Text of the "you are back on live output" announcement, held separately
  // from `copyModeRequested` on purpose: content derived from that flag alone
  // would claim a return on first mount, before any exit ever happened. Empty
  // until a return has genuinely occurred in *this* mounted terminal.
  const [exitAnnouncement, setExitAnnouncement] = useState("")
  // Mirrors `copyModeRequested` for the xterm data handler, which owns the
  // terminal instance and cannot list the state in its deps (that would remount
  // the terminal on every scrollback toggle) and would otherwise read a stale
  // closure. Kept in step below, after the render-phase resets.
  const copyModeRef = useRef(copyModeRequested)
  // The Scrollback control survives leaving scrollback, unlike the Exit button
  // that was focused; focus is handed back to it so keyboard navigation does
  // not restart from <body>.
  const scrollbackButtonRef = useRef<HTMLButtonElement | null>(null)
  const draft = useTuiDraft(name, "terminal", persistDraft)
  const workspaceSurface = surface === "workspace"
  const liveUpload = useSendFiles({
    name,
    daemon,
    onUploaded: (paths) => {
      draft.append(buildPathsText(paths))
      setModalOpen(true)
    },
  })
  const absentUpload = useSendFiles({
    name,
    daemon,
    onUploaded: (paths) => toast.success(`uploaded: ${paths.join(", ")}`),
  })
  const liveDrop = useFileDropTarget(liveUpload.sendFiles)
  const absentDrop = useFileDropTarget(absentUpload.sendFiles)

  // Mount exactly one Terminal + FitAddon into the container, wire input to the
  // socket, and keep it fitted to the container via ResizeObserver + window
  // resize. Torn down on unmount. Guarded by `absent`: the container only
  // exists in the live branch, so this effect re-runs when we leave `absent`.
  useEffect(() => {
    if (absent) return
    const el = containerRef.current
    if (!el) return

    const term = new Terminal({
      convertEol: false,
      cursorBlink: true,
      fontSize: 12,
      fontFamily: MONO_FONT,
      theme: TERM_THEME,
    })
    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    // One web-links addon per mounted terminal, owned by this effect. No
    // matcher is supplied: the addon's default already recognises only explicit
    // `http://`/`https://` text, so file paths and other schemes never become
    // clickable in the first place.
    const webLinksAddon = new WebLinksAddon(openWebLink)
    term.loadAddon(webLinksAddon)
    term.open(el)
    controller.attachTerm(term)
    term.onData((d) => {
      // `q` is copy-mode's quit key and the very byte our own Exit scrollback
      // button sends, so tracking it is exactly as accurate as tracking that
      // click. Match the chunk exactly — a paste containing a q must not clear
      // the banner — and never read copyModeRequested here: this effect owns
      // the xterm instance and does not list it in its deps, so the setter is
      // called unconditionally (React bails out when the value is unchanged).
      if (d === COPY_MODE_BYTES.exit) {
        // Announce only a return that actually happened: a `q` typed in an
        // ordinary shell (leaving a pager, opening vi) must not speak. The
        // mirror ref answers "was scrollback showing when this byte arrived?"
        // without reading the state itself. The clear below stays unconditional.
        if (copyModeRef.current) setExitAnnouncement(SCROLLBACK_EXIT_ANNOUNCEMENT)
        setCopyModeRequested(false)
      }
      send(d)
    })

    const fitNow = () => {
      try {
        fitAddon.fit()
      } catch {
        // fit() throws if the element has no layout yet (e.g. hidden tab);
        // ignore and let the next ResizeObserver tick retry.
        return
      }
      if (term.cols > 0 && term.rows > 0) controller.sendResize(term.cols, term.rows)
    }

    // Deferred refit, coalesced into one animation frame. Needed because a
    // synchronous fit right after open() (or right after a layout change) can
    // measure a not-yet-laid-out container or a not-yet-measured cell size and
    // silently leave the terminal at its 80x24 default — which is what made a
    // freshly switched-to agent render into a small corner of the pane. It also
    // collapses ResizeObserver + window-resize bursts into a single fit.
    let raf = 0
    const refit = () => {
      if (raf) cancelAnimationFrame(raf)
      raf = requestAnimationFrame(() => {
        raf = 0
        fitNow()
      })
    }
    refitRef.current = refit

    fitNow()
    refit()

    const ro = new ResizeObserver(refit)
    ro.observe(el)
    window.addEventListener("resize", refit)

    return () => {
      refitRef.current = null
      if (raf) cancelAnimationFrame(raf)
      window.removeEventListener("resize", refit)
      ro.disconnect()
      webLinksAddon.dispose()
      term.dispose()
    }
    // controller.attachTerm/send/sendResize are stable useCallbacks; re-running
    // only on the agent name or the absent→live transition is intentional.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [absent, name])

  // Re-fit whenever a socket opens (agent switch, backoff reconnect, redial):
  // the mount-effect's fit can land while the ws is still CONNECTING, so its
  // resize frame is dropped and the fresh `tmux attach` client would keep the
  // size it was dialed at. Re-fitting here re-sends the true size on a live
  // socket. Cheap and idempotent when the size has not changed.
  useEffect(() => {
    if (status === "open") refitRef.current?.()
  }, [status])

  // A replacement session must never inherit a stale scrollback banner: drop
  // the requested state whenever the terminal identity changes or the session
  // goes absent. Adjusting it during render (React's documented pattern for
  // state derived from props) rather than in an effect keeps the stale banner
  // from being painted for one frame first. Unmount needs no cleanup — the
  // state dies with the component.
  const terminalIdentity = `${name}:${absent}`
  const [lastIdentity, setLastIdentity] = useState(terminalIdentity)
  if (lastIdentity !== terminalIdentity) {
    setLastIdentity(terminalIdentity)
    setCopyModeRequested(false)
    // A replacement session must not inherit the previous one's announcement
    // either — it never left scrollback, and nothing returned to live output.
    setExitAnnouncement("")
  }
  // A closed socket ended copy-mode with the session, so the banner would
  // otherwise outlive the PTY it describes. Same render-phase adjustment for
  // the same reason; the `copyModeRequested` guard is what terminates it.
  if (status === "closed" && copyModeRequested) setCopyModeRequested(false)
  // Kept in step after every render — including the two render-phase resets
  // above, which re-render with the cleared value — so the mirror equals the
  // state at every point a keystroke can arrive: a `q` typed once the socket
  // closed must not announce a return that never happened. Written from an
  // effect rather than during render because writing a ref mid-render is a lint
  // error here (and React may discard that render).
  useEffect(() => {
    copyModeRef.current = copyModeRequested
  })

  // Never write to a closed socket: the controller is gone once the PTY ended,
  // so a late toolbar click must be a no-op rather than a lost/queued write.
  const sendCopyModeKeys = (bytes: string) => {
    if (status === "closed") return
    send(bytes)
  }

  const sendModal = () => {
    const text = draft.text
    if (!text) {
      setModalOpen(false)
      return
    }
    send(text)
    draft.clear()
    setModalOpen(false)
  }

  // Paste into the (focused, empty) terminal wrapper composes instead of
  // dropping a raw multi-line paste straight onto the shell.
  const onPaste = (e: ReactClipboardEvent) => {
    const text = e.clipboardData.getData("text")
    if (!text) return
    e.preventDefault()
    draft.append(text)
    setModalOpen(true)
  }

  if (absent) {
    return (
      <div
        data-testid="tui-absent-drop-target"
        data-file-drag-active={absentDrop.dragActive}
        onDragOver={absentDrop.onDragOver}
        onDragLeave={absentDrop.onDragLeave}
        onDrop={absentDrop.onDrop}
        className={cn(
        "flex flex-col items-center justify-center gap-2 bg-zinc-950 text-zinc-400",
        workspaceSurface ? "border-0" : "rounded-md border",
        fill ? "h-full" : "h-[32rem]",
        absentDrop.dragActive && "ring-2 ring-primary",
      )}
      >
        <div>Session not running or not interactive</div>
        <div className="text-xs">
          Start the agent and enable interactive mode (Settings → Runtime config).
        </div>
        <div className="mt-2 flex items-center gap-2">
          {onStart && (
            <Button size="sm" onClick={onStart}>
              Start
            </Button>
          )}
          {/* Files still upload to the agent's cwd even with no live session,
              so the operator can pre-stage them; report the saved paths. */}
          <SendFilesButton
            name={name}
            daemon={daemon}
            onUploaded={(paths) => toast.success(`uploaded: ${paths.join(", ")}`)}
          />
        </div>
      </div>
    )
  }

  return (
    // In fill mode this is a flex child of the pane's column: `min-h-0 flex-1`
    // makes it take exactly the space left by the pane header (and shrink with
    // the window) instead of claiming 100% of the pane and overflowing it.
    <div className={cn(
      fill
        ? "flex min-h-0 flex-1 flex-col"
        : workspaceSurface
          ? "flex flex-col"
          : "space-y-2",
      fill && !workspaceSurface && "gap-2",
    )}>
      <div
        ref={containerRef}
        data-testid="tui-screen"
        data-terminal-surface={surface}
        data-file-drag-active={liveDrop.dragActive}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onPaste={onPaste}
        onDragOver={liveDrop.onDragOver}
        onDragLeave={liveDrop.onDragLeave}
        onDrop={liveDrop.onDrop}
        className={cn(
          "overflow-hidden bg-zinc-950 p-2",
          workspaceSurface ? "border-0" : "rounded-md border",
          fill ? "min-h-0 flex-1" : "h-[32rem]",
          // Mirror the old capture affordance: ring the terminal when focused.
          focused && !workspaceSurface && "ring-2 ring-primary",
          liveDrop.dragActive && "ring-2 ring-primary",
        )}
      />

      <div
        data-testid="terminal-toolbar"
        className={cn(
          "flex shrink-0 flex-wrap items-center gap-2",
          workspaceSurface && "border-t bg-background p-2",
        )}
      >
        <SendFilesButton
          name={name}
          daemon={daemon}
          onUploaded={(paths) => {
            draft.append(buildPathsText(paths))
            setModalOpen(true)
          }}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setModalOpen(true)}
        >
          <Keyboard className="size-4" />
          Compose
        </Button>
        {HOTKEYS.map((h) => (
          <Button
            key={h.label}
            type="button"
            variant="outline"
            size="sm"
            onClick={() => send(h.bytes)}
          >
            {h.label}
          </Button>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={status === "closed"}
          ref={scrollbackButtonRef}
          onClick={() => {
            sendCopyModeKeys(COPY_MODE_BYTES.enter)
            if (status !== "closed") setCopyModeRequested(true)
            // Drop a previous announcement on the way in, so the next exit is a
            // content change the live region actually re-announces.
            setExitAnnouncement("")
          }}
        >
          Scrollback
        </Button>
        {copyModeRequested && (
          <>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => sendCopyModeKeys(COPY_MODE_BYTES.pageBack)}
            >
              Page back
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => sendCopyModeKeys(COPY_MODE_BYTES.pageForward)}
            >
              Page forward
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                sendCopyModeKeys(COPY_MODE_BYTES.exit)
                setCopyModeRequested(false)
                setExitAnnouncement(SCROLLBACK_EXIT_ANNOUNCEMENT)
                // This button is about to unmount; without this the focus falls
                // back to <body> and keyboard navigation restarts from the top
                // of the document. Only this path steals focus — after `q` the
                // focus belongs to the terminal and must stay there.
                scrollbackButtonRef.current?.focus()
              }}
            >
              Exit scrollback
            </Button>
          </>
        )}
        {/* Persistent: a live region that unmounts announces nothing, so both
            entering and leaving scrollback are content changes inside this one
            element. Outside scrollback it is sr-only, which takes it out of the
            flex flow entirely — the live-mode toolbar gains no visible chrome,
            and the announcement is spoken without being seen. */}
        <div
          role="status"
          aria-live="polite"
          className={
            copyModeRequested
              ? "flex min-w-0 flex-col text-xs text-muted-foreground"
              : "sr-only"
          }
        >
          {copyModeRequested && (
            <>
              <span className="font-medium text-foreground">Viewing scrollback</span>
              <span>{SCROLLBACK_HINT}</span>
            </>
          )}
          {exitAnnouncement && <span className="sr-only">{exitAnnouncement}</span>}
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          aria-label="help"
          title="Help"
          onClick={() => setHelpOpen(true)}
        >
          <HelpCircle className="size-4" />
        </Button>
      </div>

      <Dialog open={helpOpen} onOpenChange={setHelpOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Terminal help</DialogTitle>
            <DialogDescription>{HELP_TEXT}</DialogDescription>
          </DialogHeader>
        </DialogContent>
      </Dialog>

      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent className="max-h-[90vh]">
          <DialogHeader>
            <DialogTitle>Send text to the terminal</DialogTitle>
            <DialogDescription>
              The text is injected as-is, with no trailing Enter — submit it yourself in
              the terminal.
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={draft.text}
            onChange={(e) => draft.setText(e.target.value)}
            className="min-h-40 max-h-[60vh] overflow-auto font-mono text-sm"
            aria-label="Text to inject"
          />
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                draft.clear()
                setModalOpen(false)
              }}
            >
              Cancel
            </Button>
            <Button onClick={sendModal}>Send</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
