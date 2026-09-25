import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { readUiMode, UI_MODE_KEY, useUiMode, writeUiMode } from "./uiMode";

beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe("uiMode", () => {
  it("defaults to simple when nothing or something unknown is stored", () => {
    expect(readUiMode()).toBe("simple");
    localStorage.setItem(UI_MODE_KEY, "garbage");
    expect(readUiMode()).toBe("simple");
  });

  it("persists expert and tells every mounted reader at once", () => {
    const { result } = renderHook(() => useUiMode());
    expect(result.current).toBe("simple");
    act(() => writeUiMode("expert"));
    expect(localStorage.getItem(UI_MODE_KEY)).toBe("expert");
    expect(result.current).toBe("expert");
  });

  it("falls back to simple when storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("denied"); });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("denied"); });
    expect(() => writeUiMode("expert")).not.toThrow();
    expect(readUiMode()).toBe("simple");
  });
});
