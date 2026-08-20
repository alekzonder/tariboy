import { act, renderHook } from "@testing-library/react"
import { beforeEach, describe, expect, it } from "vitest"
import {
  DEFAULT_TASK_DETAIL_WIDTH,
  DEFAULT_TASK_NAVIGATION_WIDTH,
  MAX_TASK_DETAIL_WIDTH,
  MIN_TASK_NAVIGATION_WIDTH,
  readTaskPanelWidths,
  TASK_PANEL_WIDTHS_KEY,
  useTaskPanelWidths,
} from "./useTaskPanelWidths"

beforeEach(() => localStorage.clear())

describe("task panel width persistence", () => {
  it("defaults when the versioned record is malformed or incomplete", () => {
    for (const value of ["{", JSON.stringify({ schemaVersion: 2 }), JSON.stringify({ schemaVersion: 1, navigationWidth: 240 })]) {
      localStorage.setItem(TASK_PANEL_WIDTHS_KEY, value)
      expect(readTaskPanelWidths()).toEqual({
        navigationWidth: DEFAULT_TASK_NAVIGATION_WIDTH,
        detailWidth: DEFAULT_TASK_DETAIL_WIDTH,
      })
    }
  })

  it("clamps persisted values to usable desktop limits", () => {
    localStorage.setItem(TASK_PANEL_WIDTHS_KEY, JSON.stringify({
      schemaVersion: 1,
      navigationWidth: -50,
      detailWidth: 5000,
    }))

    expect(readTaskPanelWidths()).toEqual({
      navigationWidth: MIN_TASK_NAVIGATION_WIDTH,
      detailWidth: MAX_TASK_DETAIL_WIDTH,
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
