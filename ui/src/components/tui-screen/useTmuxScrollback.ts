import { useCallback, useEffect, useRef, useState } from "react";
import type { TerminalSocketController } from "@/hooks/useTerminalSocket";

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
} as const;

// Announced when the operator actually returns to live output. It renders
// inside the toolbar, and ui/tests/desktop/tmux-scrollback.pw.ts asserts the
// whole toolbar's text contains neither "Viewing scrollback" nor "Exit
// scrollback" once scrollback is left — so this wording must keep containing
// neither substring. That spec is the only independent check on it.
const SCROLLBACK_EXIT_ANNOUNCEMENT = "Returned to live output";

/** Requested tmux copy-mode ("scrollback") state for one terminal screen,
 * plus the toolbar handlers and the stable xterm input handler that track it.
 * `onTerminalInput` only observes bytes; the caller still sends them. */
export function useTmuxScrollback({
  name,
  absent,
  status,
  send,
}: Pick<TerminalSocketController, "name" | "absent" | "status" | "send">) {
  // We asked tmux to enter copy-mode; there is no authoritative tmux mode event
  // on this socket, so this tracks the *requested* state only and every label
  // reads as a request ("Viewing scrollback"), never as verified tmux state.
  // Cleared on Exit scrollback, on `q` typed in the terminal, when the socket
  // closes, when the terminal identity changes, when the session goes absent,
  // and on unmount. Those are not every way out of copy-mode: leaving it any
  // other way still strands the banner — notably Escape, which cancels under
  // tmux's default emacs table (including via the toolbar's own Esc hotkey) but
  // only clears the selection under copy-mode-vi, so we cannot infer it.
  const [copyModeRequested, setCopyModeRequested] = useState(false);
  // Text of the "you are back on live output" announcement, held separately
  // from `copyModeRequested` on purpose: content derived from that flag alone
  // would claim a return on first mount, before any exit ever happened. Empty
  // until a return has genuinely occurred in *this* mounted terminal.
  const [exitAnnouncement, setExitAnnouncement] = useState("");
  // Mirrors `copyModeRequested` for the xterm data handler, which owns the
  // terminal instance and cannot list the state in its deps (that would remount
  // the terminal on every scrollback toggle) and would otherwise read a stale
  // closure. Kept in step below, after the render-phase resets.
  const copyModeRef = useRef(copyModeRequested);
  // The Scrollback control survives leaving scrollback, unlike the Exit button
  // that was focused; focus is handed back to it so keyboard navigation does
  // not restart from <body>.
  const scrollbackButtonRef = useRef<HTMLButtonElement | null>(null);

  // A replacement session must never inherit a stale scrollback banner: drop
  // the requested state whenever the terminal identity changes or the session
  // goes absent. Adjusting it during render (React's documented pattern for
  // state derived from props) rather than in an effect keeps the stale banner
  // from being painted for one frame first. Unmount needs no cleanup — the
  // state dies with the component.
  const terminalIdentity = `${name}:${absent}`;
  const [lastIdentity, setLastIdentity] = useState(terminalIdentity);
  if (lastIdentity !== terminalIdentity) {
    setLastIdentity(terminalIdentity);
    setCopyModeRequested(false);
    // A replacement session must not inherit the previous one's announcement
    // either — it never left scrollback, and nothing returned to live output.
    setExitAnnouncement("");
  }
  // A closed socket ended copy-mode with the session, so the banner would
  // otherwise outlive the PTY it describes. Same render-phase adjustment for
  // the same reason; the `copyModeRequested` guard is what terminates it.
  if (status === "closed" && copyModeRequested) setCopyModeRequested(false);
  // Kept in step after every render — including the two render-phase resets
  // above, which re-render with the cleared value — so the mirror equals the
  // state at every point a keystroke can arrive: a `q` typed once the socket
  // closed must not announce a return that never happened. Written from an
  // effect rather than during render because writing a ref mid-render is a lint
  // error here (and React may discard that render).
  useEffect(() => {
    copyModeRef.current = copyModeRequested;
  });

  // Handed to the xterm mount effect, so it must never change identity: it
  // touches only state setters and the mirror ref, all stable, and therefore
  // lists no deps. A changing handler would recreate the terminal.
  const onTerminalInput = useCallback((d: string) => {
    // `q` is copy-mode's quit key and the very byte our own Exit scrollback
    // button sends, so tracking it is exactly as accurate as tracking that
    // click. Match the chunk exactly — a paste containing a q must not clear
    // the banner — and never read copyModeRequested here: the mount effect
    // owns the xterm instance and does not list it in its deps, so the setter
    // is called unconditionally (React bails out when the value is unchanged).
    if (d !== COPY_MODE_BYTES.exit) return;
    // Announce only a return that actually happened: a `q` typed in an
    // ordinary shell (leaving a pager, opening vi) must not speak. The
    // mirror ref answers "was scrollback showing when this byte arrived?"
    // without reading the state itself. The clear below stays unconditional.
    if (copyModeRef.current) setExitAnnouncement(SCROLLBACK_EXIT_ANNOUNCEMENT);
    setCopyModeRequested(false);
  }, []);

  // Never write to a closed socket: the controller is gone once the PTY ended,
  // so a late toolbar click must be a no-op rather than a lost/queued write.
  const sendCopyModeKeys = (bytes: string) => {
    if (status === "closed") return;
    send(bytes);
  };

  const enterScrollback = () => {
    sendCopyModeKeys(COPY_MODE_BYTES.enter);
    if (status !== "closed") setCopyModeRequested(true);
    // Drop a previous announcement on the way in, so the next exit is a
    // content change the live region actually re-announces.
    setExitAnnouncement("");
  };

  const exitScrollback = () => {
    sendCopyModeKeys(COPY_MODE_BYTES.exit);
    setCopyModeRequested(false);
    setExitAnnouncement(SCROLLBACK_EXIT_ANNOUNCEMENT);
    // This button is about to unmount; without this the focus falls
    // back to <body> and keyboard navigation restarts from the top
    // of the document. Only this path steals focus — after `q` the
    // focus belongs to the terminal and must stay there.
    scrollbackButtonRef.current?.focus();
  };

  return {
    copyModeRequested,
    exitAnnouncement,
    scrollbackButtonRef,
    onTerminalInput,
    enterScrollback,
    exitScrollback,
    pageBack: () => sendCopyModeKeys(COPY_MODE_BYTES.pageBack),
    pageForward: () => sendCopyModeKeys(COPY_MODE_BYTES.pageForward),
  };
}

export type TmuxScrollback = ReturnType<typeof useTmuxScrollback>;
