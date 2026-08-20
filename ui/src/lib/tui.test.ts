import { describe, it, expect } from "vitest"
import {
  buildPathsText,
  appendDraft,
} from "@/lib/tui"

describe("buildPathsText / appendDraft", () => {
  it("one path per line", () => {
    expect(buildPathsText(["/a", "/b"])).toBe("/a\n/b")
  })
  it("appends with a newline separator", () => {
    expect(appendDraft("x", "y")).toBe("x\ny")
    expect(appendDraft("", "y")).toBe("y")
  })
})
