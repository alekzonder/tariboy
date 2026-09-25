import { useEffect, useRef } from "react"
import { defaultTaskDetailWidth, MIN_TASK_DETAIL_WIDTH } from "./useTaskPanelWidths"

export default function TaskPanelResizeHandle({
  width,
  maximum,
  workspaceRef,
  onResize,
}: {
  width: number
  maximum: number
  // The panel's right edge is the workspace's; without one it is the window's.
  workspaceRef?: React.RefObject<HTMLDivElement | null>
  onResize: (width: number) => void
}) {
  const cleanupDragRef = useRef<(() => void) | null>(null)
  const minimum = MIN_TASK_DETAIL_WIDTH
  const defaultWidth = defaultTaskDetailWidth()
  const label = "Resize task details"
  const resize = (requestedWidth: number) => {
    onResize(Math.min(requestedWidth, maximum))
  }

  useEffect(() => () => cleanupDragRef.current?.(), [])

  const startDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    if ((Number.isFinite(event.button) && event.button !== 0) || event.isPrimary === false) return
    event.preventDefault()
    cleanupDragRef.current?.()
    const pointerId = event.pointerId
    const target = event.currentTarget
    let finished = false
    const isDifferentPointer = (candidate: number) => (
      Number.isFinite(pointerId) && Number.isFinite(candidate) && candidate !== pointerId
    )
    const onMove = (moveEvent: PointerEvent) => {
      if (isDifferentPointer(moveEvent.pointerId)) return
      const right = workspaceRef
        ? workspaceRef.current?.getBoundingClientRect().right
        : window.innerWidth
      if (right === undefined) return
      resize(right - moveEvent.clientX)
    }
    const cleanup = () => {
      if (finished) return
      finished = true
      cleanupDragRef.current = null
      window.removeEventListener("pointermove", onMove)
      window.removeEventListener("pointerup", onEnd)
      window.removeEventListener("pointercancel", onEnd)
      window.removeEventListener("blur", cleanup)
      document.removeEventListener("visibilitychange", onVisibilityChange)
      target.removeEventListener("lostpointercapture", onLostPointerCapture)
      document.body.style.removeProperty("cursor")
      document.body.style.removeProperty("user-select")
      try {
        if (target.hasPointerCapture?.(pointerId)) target.releasePointerCapture(pointerId)
      } catch {
        // The WebView may already have released capture during cancellation.
      }
    }
    const onEnd = (endEvent: PointerEvent) => {
      if (!isDifferentPointer(endEvent.pointerId)) cleanup()
    }
    const onLostPointerCapture = (lostEvent: PointerEvent) => {
      if (!isDifferentPointer(lostEvent.pointerId)) cleanup()
    }
    const onVisibilityChange = () => {
      if (document.visibilityState !== "visible") cleanup()
    }
    window.addEventListener("pointermove", onMove)
    window.addEventListener("pointerup", onEnd)
    window.addEventListener("pointercancel", onEnd)
    window.addEventListener("blur", cleanup)
    document.addEventListener("visibilitychange", onVisibilityChange)
    target.addEventListener("lostpointercapture", onLostPointerCapture)
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
    const step = (event.shiftKey ? 32 : 8) * -1
    if (event.key === "ArrowLeft") {
      event.preventDefault()
      resize(width - step)
    } else if (event.key === "ArrowRight") {
      event.preventDefault()
      resize(width + step)
    } else if (event.key === "Home") {
      event.preventDefault()
      resize(defaultWidth)
    }
  }

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={width}
      aria-valuemin={minimum}
      aria-valuemax={maximum}
      tabIndex={0}
      className="task-panel-resize-handle"
      onPointerDown={startDrag}
      onKeyDown={onKeyDown}
      onDoubleClick={() => resize(defaultWidth)}
    />
  )
}
