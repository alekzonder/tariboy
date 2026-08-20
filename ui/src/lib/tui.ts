import { useCallback, useState } from "react"

/** localStorage key holding the retained inject-modal draft for one agent's
 * section. Keyed by agent name AND section so the manager and worker drafts
 * never clobber each other. */
export function tuiDraftKey(name: string, section: string): string {
  return `tui:draft:${name}:${section}`
}

/** Append `extra` to a draft body so it lands at the END, separated from the
 * existing text by a single newline. Empty `extra` is a no-op; appending to an
 * empty draft yields `extra` verbatim. */
export function appendDraft(prev: string, extra: string): string {
  if (!extra) return prev
  if (!prev) return extra
  return prev.endsWith("\n") ? prev + extra : prev + "\n" + extra
}

/** An inject-modal draft for one agent's section. Persistent callers keep the
 * body in `localStorage` (keyed via {@link tuiDraftKey}) across modal closes and
 * page reloads. Workspace callers opt out, keeping the body only in component
 * memory. Cleared on explicit Cancel or a successful Send. */
export function useTuiDraft(name: string, section: string, persistent = true) {
  const key = tuiDraftKey(name, section)
  const [text, setTextState] = useState<string>(
    () => persistent ? (localStorage.getItem(key) ?? "") : "",
  )
  const persist = useCallback(
    (next: string) => {
      if (!persistent) return
      if (next) localStorage.setItem(key, next)
      else localStorage.removeItem(key)
    },
    [key, persistent],
  )
  const setText = useCallback(
    (next: string) => {
      setTextState(next)
      persist(next)
    },
    [persist],
  )
  const append = useCallback(
    (extra: string) =>
      setTextState((prev) => {
        const next = appendDraft(prev, extra)
        persist(next)
        return next
      }),
    [persist],
  )
  const clear = useCallback(() => setText(""), [setText])
  return { text, setText, append, clear }
}

/** Build the modal body for a set of uploaded files: one full server path per
 * line, in upload order. */
export function buildPathsText(paths: string[]): string {
  return paths.join("\n")
}
