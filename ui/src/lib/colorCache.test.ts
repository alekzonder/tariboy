import { describe, it, expect, beforeEach } from "vitest"
import {
  COLOR_CACHE_TTL_MS,
  colorCacheKey,
  getCachedColor,
  removeCachedColor,
  setCachedColor,
} from "@/lib/colorCache"

beforeEach(() => localStorage.clear())

describe("colorCache helper", () => {
  it("round-trips a stored color (set then get)", () => {
    const now = 1_000_000
    setCachedColor("alpha", "#0000ff", now)
    expect(getCachedColor("alpha", now)).toBe("#0000ff")
  })

  it("normalizes the stored hex to lowercase", () => {
    const now = 1_000_000
    setCachedColor("alpha", "#00FF00", now)
    expect(getCachedColor("alpha", now)).toBe("#00ff00")
  })

  it("returns null for a missing entry", () => {
    expect(getCachedColor("nobody")).toBeNull()
  })

  it("keys entries per agent name", () => {
    const now = 1_000_000
    setCachedColor("alpha", "#0000ff", now)
    setCachedColor("beta", "#ff8000", now)
    expect(getCachedColor("alpha", now)).toBe("#0000ff")
    expect(getCachedColor("beta", now)).toBe("#ff8000")
    expect(colorCacheKey("alpha")).not.toBe(colorCacheKey("beta"))
  })

  it("honours the 1h TTL: a fresh entry is valid, an expired one is ignored", () => {
    const ts = 1_000_000
    setCachedColor("alpha", "#0000ff", ts)
    // Just under the TTL → still valid.
    expect(getCachedColor("alpha", ts + COLOR_CACHE_TTL_MS - 1)).toBe("#0000ff")
    // At / past the TTL → ignored.
    expect(getCachedColor("alpha", ts + COLOR_CACHE_TTL_MS)).toBeNull()
    expect(getCachedColor("alpha", ts + COLOR_CACHE_TTL_MS + 5000)).toBeNull()
  })

  it("reset-on-update: a later set overwrites the value and refreshes the timestamp", () => {
    const t0 = 1_000_000
    setCachedColor("alpha", "#0000ff", t0)
    // Update near the end of the original TTL window with a new color.
    const t1 = t0 + COLOR_CACHE_TTL_MS - 1
    setCachedColor("alpha", "#ff8000", t1)
    expect(getCachedColor("alpha", t1)).toBe("#ff8000")
    // The timestamp was refreshed: still valid well past the ORIGINAL expiry.
    expect(getCachedColor("alpha", t0 + COLOR_CACHE_TTL_MS + 1000)).toBe("#ff8000")
  })

  it("never caches an invalid hex", () => {
    setCachedColor("alpha", "not-a-color")
    setCachedColor("alpha", "#fff")
    expect(getCachedColor("alpha")).toBeNull()
  })

  it("removeCachedColor evicts an entry so a cleared color no longer repaints", () => {
    const now = 1_000_000
    setCachedColor("alpha", "#0000ff", now)
    expect(getCachedColor("alpha", now)).toBe("#0000ff")
    removeCachedColor("alpha")
    expect(getCachedColor("alpha", now)).toBeNull()
  })

  it("removeCachedColor on a missing entry is a no-op (no throw)", () => {
    expect(() => removeCachedColor("nobody")).not.toThrow()
    expect(getCachedColor("nobody")).toBeNull()
  })

  it("ignores a malformed / non-hex stored entry", () => {
    localStorage.setItem(colorCacheKey("alpha"), "{ not json")
    expect(getCachedColor("alpha")).toBeNull()
    localStorage.setItem(colorCacheKey("alpha"), JSON.stringify({ color: "blue", ts: Date.now() }))
    expect(getCachedColor("alpha")).toBeNull()
    localStorage.setItem(colorCacheKey("alpha"), JSON.stringify({ color: "#0000ff" }))
    expect(getCachedColor("alpha")).toBeNull()
  })
})
