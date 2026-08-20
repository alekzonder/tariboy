import { useCallback, useRef, useState } from "react"

export const TASK_PANEL_WIDTHS_KEY = "tasks:workspace:v1"
export const TASK_PANEL_WIDTHS_SCHEMA_VERSION = 1 as const

export const DEFAULT_TASK_NAVIGATION_WIDTH = 208
export const MIN_TASK_NAVIGATION_WIDTH = 160
export const MAX_TASK_NAVIGATION_WIDTH = 360
export const DEFAULT_TASK_DETAIL_WIDTH = 410
export const MIN_TASK_DETAIL_WIDTH = 320
export const MAX_TASK_DETAIL_WIDTH = 640

export type TaskPanelWidths = {
  navigationWidth: number
  detailWidth: number
}

const defaults: TaskPanelWidths = {
  navigationWidth: DEFAULT_TASK_NAVIGATION_WIDTH,
  detailWidth: DEFAULT_TASK_DETAIL_WIDTH,
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, Math.round(value)))
}

export function clampTaskNavigationWidth(value: number): number {
  return clamp(value, MIN_TASK_NAVIGATION_WIDTH, MAX_TASK_NAVIGATION_WIDTH)
}

export function clampTaskDetailWidth(value: number): number {
  return clamp(value, MIN_TASK_DETAIL_WIDTH, MAX_TASK_DETAIL_WIDTH)
}

export function readTaskPanelWidths(): TaskPanelWidths {
  try {
    const raw = globalThis.localStorage?.getItem(TASK_PANEL_WIDTHS_KEY)
    if (!raw) return defaults
    const value = JSON.parse(raw) as Record<string, unknown>
    if (
      value.schemaVersion !== TASK_PANEL_WIDTHS_SCHEMA_VERSION
      || typeof value.navigationWidth !== "number"
      || !Number.isFinite(value.navigationWidth)
      || typeof value.detailWidth !== "number"
      || !Number.isFinite(value.detailWidth)
    ) return defaults
    return {
      navigationWidth: clampTaskNavigationWidth(value.navigationWidth),
      detailWidth: clampTaskDetailWidth(value.detailWidth),
    }
  } catch {
    return defaults
  }
}

function persistTaskPanelWidths(widths: TaskPanelWidths): void {
  try {
    globalThis.localStorage?.setItem(TASK_PANEL_WIDTHS_KEY, JSON.stringify({
      schemaVersion: TASK_PANEL_WIDTHS_SCHEMA_VERSION,
      ...widths,
    }))
  } catch {
    // Web Storage is a best-effort Desktop convenience.
  }
}

export function useTaskPanelWidths(): TaskPanelWidths & {
  setNavigationWidth: (value: number) => void
  setDetailWidth: (value: number) => void
} {
  const [widths, setWidths] = useState(readTaskPanelWidths)
  const widthsRef = useRef(widths)
  const update = useCallback((next: Partial<TaskPanelWidths>) => {
    const value = { ...widthsRef.current, ...next }
    widthsRef.current = value
    setWidths(value)
    persistTaskPanelWidths(value)
  }, [])
  const setNavigationWidth = useCallback((value: number) => {
    update({ navigationWidth: clampTaskNavigationWidth(value) })
  }, [update])
  const setDetailWidth = useCallback((value: number) => {
    update({ detailWidth: clampTaskDetailWidth(value) })
  }, [update])
  return { ...widths, setNavigationWidth, setDetailWidth }
}
