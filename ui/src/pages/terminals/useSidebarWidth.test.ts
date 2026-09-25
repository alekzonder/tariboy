import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import {
  useSidebarState, useSidebarWidth, readSidebarWidth, clampSidebarWidth,
  SIDEBAR_STATE_KEY, LEGACY_WORKSPACE_STATE_KEY,
  DEFAULT_SIDEBAR_WIDTH, MIN_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH,
} from "./useSidebarWidth";

beforeEach(() => localStorage.clear());

describe("readSidebarWidth", () => {
  it("defaults when unset", () => {
    expect(readSidebarWidth()).toBe(DEFAULT_SIDEBAR_WIDTH);
  });
  it("defaults on garbage", () => {
    localStorage.setItem(SIDEBAR_STATE_KEY, "wide please");
    expect(readSidebarWidth()).toBe(DEFAULT_SIDEBAR_WIDTH);
  });
  it("clamps a stored out-of-range value", () => {
    localStorage.setItem(SIDEBAR_STATE_KEY, JSON.stringify({ width: 5000, hidden: false }));
    expect(readSidebarWidth()).toBe(MAX_SIDEBAR_WIDTH);
  });
  it("keeps the sidebar recorded by the removed terminal canvas", () => {
    localStorage.setItem(LEGACY_WORKSPACE_STATE_KEY, JSON.stringify({
      schemaVersion: 1, layout: {}, activeTerminal: null, sidebar: { width: 333, hidden: true },
    }));
    const { result } = renderHook(() => useSidebarState());
    expect(result.current).toMatchObject({ width: 333, hidden: true });
  });
});

describe("clampSidebarWidth", () => {
  it("clamps both ends and rounds", () => {
    expect(clampSidebarWidth(10)).toBe(MIN_SIDEBAR_WIDTH);
    expect(clampSidebarWidth(9999)).toBe(MAX_SIDEBAR_WIDTH);
    expect(clampSidebarWidth(300.6)).toBe(301);
  });
});

describe("useSidebarWidth", () => {
  it("persists the new width and restores it on the next mount", () => {
    const first = renderHook(() => useSidebarWidth());
    act(() => first.result.current[1](420));
    expect(first.result.current[0]).toBe(420);
    expect(JSON.parse(localStorage.getItem(SIDEBAR_STATE_KEY)!))
      .toEqual({ width: 420, hidden: false });

    // Fresh mount = a reload: the width comes back from storage.
    const second = renderHook(() => useSidebarWidth());
    expect(second.result.current[0]).toBe(420);
  });

  it("clamps what it persists", () => {
    const { result } = renderHook(() => useSidebarWidth());
    act(() => result.current[1](-100));
    expect(result.current[0]).toBe(MIN_SIDEBAR_WIDTH);
    expect(JSON.parse(localStorage.getItem(SIDEBAR_STATE_KEY)!))
      .toEqual({ width: MIN_SIDEBAR_WIDTH, hidden: false });
  });
});
