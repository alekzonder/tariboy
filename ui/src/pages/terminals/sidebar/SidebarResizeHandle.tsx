import type { RefObject } from "react";
import { DEFAULT_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH, MIN_SIDEBAR_WIDTH } from "../useSidebarWidth";

/** The 8px separator that resizes the sidebar by pointer drag, arrow keys,
 *  Home, or a double click back to the default width. */
export function SidebarResizeHandle({
  asideRef,
  width,
  onResize,
}: {
  asideRef: RefObject<HTMLElement | null>;
  width: number;
  onResize: (px: number) => void;
}) {
  // Drag state lives on window listeners rather than pointer capture: capture
  // is spotty in jsdom, and window listeners keep tracking the drag when the
  // pointer leaves the 4px handle (which it always does).
  const startDrag = (e: React.PointerEvent) => {
    e.preventDefault();
    const left = asideRef.current?.getBoundingClientRect().left ?? 0;
    const onMove = (ev: PointerEvent) => onResize(ev.clientX - left);
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      document.body.style.removeProperty("cursor");
      document.body.style.removeProperty("user-select");
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    // Keep the resize cursor (and kill text selection) for the whole drag, not
    // just while the pointer is over the handle.
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const step = e.shiftKey ? 32 : 8;
    if (e.key === "ArrowLeft") {
      e.preventDefault();
      onResize(width - step);
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      onResize(width + step);
    } else if (e.key === "Home") {
      e.preventDefault();
      onResize(DEFAULT_SIDEBAR_WIDTH);
    }
  };

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="resize sidebar"
      aria-valuenow={width}
      aria-valuemin={MIN_SIDEBAR_WIDTH}
      aria-valuemax={MAX_SIDEBAR_WIDTH}
      tabIndex={0}
      onPointerDown={startDrag}
      onKeyDown={onKeyDown}
      onDoubleClick={() => onResize(DEFAULT_SIDEBAR_WIDTH)}
      className="w-2 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-primary/30 focus-visible:bg-primary/30 focus-visible:outline-none"
    />
  );
}
