import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import type { TerminalSocketController } from "@/hooks/useTerminalSocket";
import { isAndroidShell, openExternalUrl } from "@/lib/desktop";

// Tailwind's default `font-mono` stack — mirrored here so the xterm canvas
// renders in the same monospace face as the rest of the app.
const MONO_FONT =
  'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace';

// Zinc dark palette matching the old snapshot terminal (bg zinc-950, fg zinc-100).
const TERM_THEME = {
  background: "#09090b",
  foreground: "#e4e4e7",
  cursor: "#e4e4e7",
  cursorAccent: "#09090b",
  selectionBackground: "#3f3f46",
};

// Failure text for an external open. Fixed on purpose: the terminal line — and
// therefore any token in its query string — must never reach a toast or a log.
const WEB_LINK_ERROR = "Could not open web link";

/** Open a Command-clicked terminal link in the operator's browser.
 *
 * Every other click is left entirely to xterm: no `preventDefault`,
 * `stopPropagation`, or focus call, so selection, mouse reporting and focus
 * behave exactly as they did before. The URL is parsed and its scheme checked
 * here, and again natively (`open_external_url` on Desktop, the navigation
 * guard on Android) — this side is a usability filter, not the authorization
 * boundary. A phone has no Command key, so in the Android app a tap on the
 * link opens it. */
async function openWebLink(event: MouseEvent, text: string) {
  if (!event.metaKey && !isAndroidShell()) return;
  let url: URL;
  try {
    url = new URL(text);
  } catch {
    // Not an absolute URL (bare host, path, protocol-relative, malformed).
    return;
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") return;
  try {
    await openExternalUrl(url.toString());
  } catch {
    toast.error(WEB_LINK_ERROR);
  }
}

/** Mount one live xterm.js terminal into the returned container ref, driven by
 * the socket controller. `onInput` sees every keystroke chunk before it is
 * sent; it is listed in the mount effect's deps, so it must be stable (a
 * changing handler would recreate the terminal). */
export function useXtermTerminal(
  { absent, name, send, sendResize, attachTerm, status }: TerminalSocketController,
  onInput: (d: string) => void,
) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  // Set by the mount effect below; lets other effects (re)fit the live terminal
  // without owning its lifetime. Null while no terminal is mounted.
  const refitRef = useRef<(() => void) | null>(null);

  // Mount exactly one Terminal + FitAddon into the container, wire input to the
  // socket, and keep it fitted to the container via ResizeObserver + window
  // resize. Torn down on unmount. Guarded by `absent`: the container only
  // exists in the live branch, so this effect re-runs when we leave `absent`.
  useEffect(() => {
    if (absent) return;
    const el = containerRef.current;
    if (!el) return;

    const term = new Terminal({
      convertEol: false,
      cursorBlink: true,
      fontSize: 12,
      fontFamily: MONO_FONT,
      theme: TERM_THEME,
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    // One web-links addon per mounted terminal, owned by this effect. No
    // matcher is supplied: the addon's default already recognises only explicit
    // `http://`/`https://` text, so file paths and other schemes never become
    // clickable in the first place.
    const webLinksAddon = new WebLinksAddon(openWebLink);
    term.loadAddon(webLinksAddon);
    term.open(el);
    attachTerm(term);
    term.onData((d) => {
      onInput(d);
      send(d);
    });

    const fitNow = () => {
      try {
        fitAddon.fit();
      } catch {
        // fit() throws if the element has no layout yet (e.g. hidden tab);
        // ignore and let the next ResizeObserver tick retry.
        return;
      }
      if (term.cols > 0 && term.rows > 0) sendResize(term.cols, term.rows);
    };

    // Deferred refit, coalesced into one animation frame. Needed because a
    // synchronous fit right after open() (or right after a layout change) can
    // measure a not-yet-laid-out container or a not-yet-measured cell size and
    // silently leave the terminal at its 80x24 default — which is what made a
    // freshly switched-to agent render into a small corner of the pane. It also
    // collapses ResizeObserver + window-resize bursts into a single fit.
    let raf = 0;
    const refit = () => {
      if (raf) cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        raf = 0;
        fitNow();
      });
    };
    refitRef.current = refit;

    fitNow();
    refit();

    const ro = new ResizeObserver(refit);
    ro.observe(el);
    window.addEventListener("resize", refit);

    return () => {
      refitRef.current = null;
      if (raf) cancelAnimationFrame(raf);
      window.removeEventListener("resize", refit);
      ro.disconnect();
      webLinksAddon.dispose();
      term.dispose();
    };
    // attachTerm/send/sendResize are stable useCallbacks and onInput is a
    // dep-free useCallback, so this re-runs only on the agent name or the
    // absent→live transition.
  }, [absent, name, attachTerm, send, sendResize, onInput]);

  // Re-fit whenever a socket opens (agent switch, backoff reconnect, redial):
  // the mount-effect's fit can land while the ws is still CONNECTING, so its
  // resize frame is dropped and the fresh `tmux attach` client would keep the
  // size it was dialed at. Re-fitting here re-sends the true size on a live
  // socket. Cheap and idempotent when the size has not changed.
  useEffect(() => {
    if (status === "open") refitRef.current?.();
  }, [status]);

  return containerRef;
}
