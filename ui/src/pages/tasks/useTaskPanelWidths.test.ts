import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  defaultTaskDetailWidth,
  DEFAULT_TASK_NAVIGATION_WIDTH,
  MIN_TASK_NAVIGATION_WIDTH,
  readTaskPanelWidths,
  TASK_PANEL_WIDTHS_KEY,
  useTaskPanelWidths,
} from "./useTaskPanelWidths"

beforeEach(() => localStorage.clear())
afterEach(() => vi.unstubAllGlobals())

describe("task panel width persistence", () => {
  it("defaults the detail sheet to half of an ultrawide viewport", () => {
    vi.stubGlobal("innerWidth", 3440)

    expect(defaultTaskDetailWidth()).toBe(1720)
  })

  it("defaults when the versioned record is malformed or incomplete", () => {
    for (const value of ["{", JSON.stringify({ schemaVersion: 2 }), JSON.stringify({ schemaVersion: 1, navigationWidth: 240 })]) {
      localStorage.setItem(TASK_PANEL_WIDTHS_KEY, value)
      expect(readTaskPanelWidths()).toEqual({
        navigationWidth: DEFAULT_TASK_NAVIGATION_WIDTH,
        detailWidth: defaultTaskDetailWidth(),
      })
    }
  })

  it("clamps persisted values to usable minimums", () => {
    vi.stubGlobal("innerWidth", 1000)
    localStorage.setItem(TASK_PANEL_WIDTHS_KEY, JSON.stringify({
      schemaVersion: 1,
      navigationWidth: -50,
      detailWidth: 5000,
    }))

    expect(readTaskPanelWidths()).toEqual({
      navigationWidth: MIN_TASK_NAVIGATION_WIDTH,
      detailWidth: 920,
    })
  })

  it("keeps both widths when sequential updates persist", () => {
    const { result } = renderHook(() => useTaskPanelWidths())

    act(() => {
      result.current.setNavigationWidth(260)
      result.current.setDetailWidth(480)
    })

    expect(result.current).toMatchObject({ navigationWidth: 260, detailWidth: 480 })
    expect(JSON.parse(localStorage.getItem(TASK_PANEL_WIDTHS_KEY) ?? "{}"))
      .toEqual({ schemaVersion: 1, navigationWidth: 260, detailWidth: 480 })
  })
})
