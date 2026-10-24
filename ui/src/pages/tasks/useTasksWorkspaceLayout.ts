import { useEffect, useRef, useState } from "react";
import { MIN_TASK_DETAIL_WIDTH, useTaskPanelWidths } from "./useTaskPanelWidths";

export function useTasksWorkspaceLayout() {
  const workspaceRef = useRef<HTMLDivElement | null>(null);
  const { detailWidth, setDetailWidth } = useTaskPanelWidths();
  const [workspaceWidth, setWorkspaceWidth] = useState(0);
  useEffect(() => {
    const workspace = workspaceRef.current;
    if (!workspace) return;
    const measure = () => setWorkspaceWidth(workspace.getBoundingClientRect().width);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(workspace);
    window.addEventListener("resize", measure);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);
  const detailMaximum =
    workspaceWidth > 0
      ? Math.max(MIN_TASK_DETAIL_WIDTH, workspaceWidth - 80)
      : Math.max(MIN_TASK_DETAIL_WIDTH, (globalThis.innerWidth || 820) - 80);
  const effectiveDetailWidth = Math.min(detailWidth, detailMaximum);
  return { workspaceRef, detailMaximum, effectiveDetailWidth, setDetailWidth };
}
