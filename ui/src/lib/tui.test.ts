import { afterEach, describe, it, expect } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { buildPathsText, appendDraft, useTuiDraft } from "@/lib/tui";

describe("buildPathsText / appendDraft", () => {
  it("one path per line", () => {
    expect(buildPathsText(["/a", "/b"])).toBe("/a\n/b");
  });
  it("appends with a newline separator", () => {
    expect(appendDraft("x", "y")).toBe("x\ny");
    expect(appendDraft("", "y")).toBe("y");
  });
});

describe("useTuiDraft", () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage")!;
  afterEach(() => {
    Object.defineProperty(globalThis, "localStorage", original);
    localStorage.clear();
  });

  it("keeps the draft per agent and section", () => {
    localStorage.setItem("tui:draft:worker:manager", "saved");
    const { result } = renderHook(() => useTuiDraft("worker", "manager"));
    expect(result.current.text).toBe("saved");
    act(() => result.current.setText("next"));
    expect(localStorage.getItem("tui:draft:worker:manager")).toBe("next");
    act(() => result.current.clear());
    expect(localStorage.getItem("tui:draft:worker:manager")).toBeNull();
  });

  it("keeps a draft in memory when storage is blocked", () => {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      get() {
        throw new Error("blocked");
      },
    });
    const { result } = renderHook(() => useTuiDraft("worker", "manager"));
    expect(result.current.text).toBe("");
    act(() => result.current.append("hello"));
    expect(result.current.text).toBe("hello");
  });
});
