import { act, renderHook } from "@testing-library/react"
import { expect, it } from "vitest"
import { useTasksWorkspaceView } from "./useTasksWorkspaceView"

it("starts root creation in the active view without losing expansion", () => {
  const { result } = renderHook(() => useTasksWorkspaceView())
  act(() => {
    result.current.setQuery("hidden")
    result.current.setStatusView("closed")
    result.current.setExpanded(new Set(["TEST-1"]))
  })
  act(() => result.current.startCreate())
  expect(result.current.query).toBe("")
  expect(result.current.statusView).toBe("active")
  expect(result.current.creatingParent).toBe("")
  expect(result.current.expanded.has("TEST-1")).toBe(true)
})
