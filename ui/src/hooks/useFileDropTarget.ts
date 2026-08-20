import { useCallback, useState, type DragEvent } from "react"
import { toast } from "sonner"

/** True when the drag carries files.
 *
 * `dragover` and `drop` deliberately gate on *different* things, and the two
 * must not be merged back into one helper. Outside `dragstart`/`drop` the drag
 * data store is in protected mode: `DataTransfer.files` is always empty and
 * `getAsFile()` always returns null, so a files-based check on `dragover`
 * never matches, `preventDefault()` never runs, the element never becomes a
 * valid drop target, and the browser navigates away to the dropped file.
 * `types` stays readable in protected mode, so `dragover` gates on that; only
 * `drop` may read the files themselves.
 *
 * The spec's `types` getter adds "Files" (capital F) while MDN's prose says
 * "files"; the comparison is case-insensitive so it holds either way.
 *
 * This gate belongs to `dragover` only. `drop` prevents the default
 * unconditionally instead — see the comment on `onDrop` below. */
function dragCarriesFiles(event: DragEvent<HTMLElement>): boolean {
  return Array.from(event.dataTransfer.types ?? []).some(
    (type) => type.toLowerCase() === "files",
  )
}

/** The dropped files with directories removed, plus whether any were removed.
 *
 * A dropped folder appears in `DataTransfer.files` as a bogus zero-byte entry,
 * so it has to be recognised through `items[].webkitGetAsEntry().isDirectory`
 * — never by size, which would silently discard legitimate empty files. When
 * `items` or `webkitGetAsEntry` is unavailable the raw file list is used, so
 * no browser ends up worse off than before. */
function droppedFiles(event: DragEvent<HTMLElement>): {
  files: File[]
  skippedDirectory: boolean
} {
  const all = Array.from(event.dataTransfer.files ?? [])
  const fileItems = Array.from(event.dataTransfer.items ?? []).filter(
    (item) => item.kind === "file",
  )
  const inspectable =
    fileItems.length === all.length
    && fileItems.every((item) => typeof item.webkitGetAsEntry === "function")
  if (!inspectable) return { files: all, skippedDirectory: false }

  const files = all.filter((_, index) => !fileItems[index].webkitGetAsEntry()?.isDirectory)
  return { files, skippedDirectory: files.length !== all.length }
}

/** Event handlers for targets that accept only real file drags. */
export function useFileDropTarget(sendFiles: (files: File[]) => Promise<void>) {
  const [dragActive, setDragActive] = useState(false)

  const onDragOver = useCallback((event: DragEvent<HTMLElement>) => {
    if (!dragCarriesFiles(event)) return
    event.preventDefault()
    setDragActive(true)
  }, [])

  const onDragLeave = useCallback(() => setDragActive(false), [])

  const onDrop = useCallback((event: DragEvent<HTMLElement>) => {
    // Unconditional, and before anything can bail out: by the time `drop`
    // fires our own `dragover` has already claimed the drag, so the browser
    // would navigate away to the dropped item on every path that returns
    // early — including a drag that advertises "Files" but yields none
    // (promised/virtual files, a remote mount, a source that cancels late).
    // No `dragleave` follows a drop either, so the ring must be cleared here
    // too. `dragover` stays conditional for the opposite reason: preventing
    // the default there is what claims the drag, and claiming every drag
    // would suppress every other drop target on the page.
    event.preventDefault()
    setDragActive(false)
    const { files, skippedDirectory } = droppedFiles(event)
    if (skippedDirectory) toast.error("folders cannot be sent — drop files instead")
    if (files.length === 0) return
    void sendFiles(files).finally(() => setDragActive(false))
  }, [sendFiles])

  return { dragActive, onDragOver, onDragLeave, onDrop }
}
