import { useEffect, useRef } from "react"
import { clampTaskColumnWidth, defaultTaskColumnLayout, setTaskColumnWidth, type SizedTaskColumnId } from "./taskColumnLayout"

/** The right edge of a header cell. Pointer Events with capture, so a mouse
 *  and a finger drag it the same way; arrows step it, Home and a double click
 *  put the default width back. */
export default function TaskColumnResizeHandle({ id, label, width }: {
  id: SizedTaskColumnId
  label: string
  width: number
}) {
  const cleanupDragRef = useRef<(() => void) | null>(null)
  const defaultWidth = defaultTaskColumnLayout().widths[id]

  useEffect(() => () => cleanupDragRef.current?.(), [])

  const startDrag = (event: React.PointerEvent<HTMLSpanElement>) => {
    if ((Number.isFinite(event.button) && event.button !== 0) || event.isPrimary === false) return
    event.preventDefault()
    event.stopPropagation()
    cleanupDragRef.current?.()
    const pointerId = event.pointerId
    const target = event.currentTarget
    const startX = event.clientX
    const startWidth = width
    let finished = false
    const isDifferentPointer = (candidate: number) => (
      Number.isFinite(pointerId) && Number.isFinite(candidate) && candidate !== pointerId
    )
    const onMove = (moveEvent: PointerEvent) => {
      if (!isDifferentPointer(moveEvent.pointerId)) setTaskColumnWidth(id, startWidth + moveEvent.clientX - startX)
    }
    const onEnd = (endEvent: PointerEvent) => {
      if (!isDifferentPointer(endEvent.pointerId)) cleanup()
    }
    const cleanup = () => {
      if (finished) return
      finished = true
      cleanupDragRef.current = null
      window.removeEventListener("pointermove", onMove)
      window.removeEventListener("pointerup", onEnd)
      window.removeEventListener("pointercancel", onEnd)
      window.removeEventListener("blur", cleanup)
      target.removeEventListener("lostpointercapture", onEnd)
      document.body.style.removeProperty("cursor")
      document.body.style.removeProperty("user-select")
      try {
        if (target.hasPointerCapture?.(pointerId)) target.releasePointerCapture(pointerId)
      } catch {
        // The WebView may already have released capture during cancellation.
      }
    }
    window.addEventListener("pointermove", onMove)
    window.addEventListener("pointerup", onEnd)
    window.addEventListener("pointercancel", onEnd)
    window.addEventListener("blur", cleanup)
    target.addEventListener("lostpointercapture", onEnd)
    cleanupDragRef.current = cleanup
    try {
      target.setPointerCapture?.(pointerId)
    } catch {
      // Window listeners still own the session when capture is unavailable.
    }
    document.body.style.cursor = "col-resize"
    document.body.style.userSelect = "none"
  }

  const onKeyDown = (event: React.KeyboardEvent) => {
    const step = event.shiftKey ? 32 : 8
    const next = event.key === "ArrowLeft" ? width - step
      : event.key === "ArrowRight" ? width + step
        : event.key === "Home" ? defaultWidth
          : undefined
    if (next === undefined) return
    event.preventDefault()
    setTaskColumnWidth(id, next)
  }

  return (
    <span
      role="separator"
      aria-orientation="vertical"
      aria-label={`Resize ${label} column`}
      aria-valuenow={width}
      aria-valuemin={clampTaskColumnWidth(id, 0)}
      aria-valuemax={clampTaskColumnWidth(id, Infinity)}
      tabIndex={0}
      className="task-column-resize-handle"
      onPointerDown={startDrag}
      onKeyDown={onKeyDown}
      onDoubleClick={() => setTaskColumnWidth(id, defaultWidth)}
    />
  )
}
