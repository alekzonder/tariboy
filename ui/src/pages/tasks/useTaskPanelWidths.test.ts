import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  defaultTaskDetailWidth,
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
      expect(readTaskPanelWidths()).toEqual({ detailWidth: defaultTaskDetailWidth() })
    }
  })

  // The rail is gone, but a record written before it left still carries its
  // width; the detail width beside it stays readable.
  it("reads the detail width out of a record that still names the removed rail", () => {
    vi.stubGlobal("innerWidth", 1000)
    localStorage.setItem(TASK_PANEL_WIDTHS_KEY, JSON.stringify({
      schemaVersion: 1,
      navigationWidth: 260,
      detailWidth: 480,
    }))

    expect(readTaskPanelWidths()).toEqual({ detailWidth: 480 })
  })

  it("clamps persisted values to usable maximums", () => {
    vi.stubGlobal("innerWidth", 1000)
    localStorage.setItem(TASK_PANEL_WIDTHS_KEY, JSON.stringify({
      schemaVersion: 1,
      detailWidth: 5000,
    }))

    expect(readTaskPanelWidths()).toEqual({ detailWidth: 920 })
  })

  it("persists the detail width on update", () => {
    const { result } = renderHook(() => useTaskPanelWidths())

    act(() => result.current.setDetailWidth(480))

    expect(result.current).toMatchObject({ detailWidth: 480 })
    expect(JSON.parse(localStorage.getItem(TASK_PANEL_WIDTHS_KEY) ?? "{}"))
      .toEqual({ schemaVersion: 1, detailWidth: 480 })
  })
})
