import { describe, it, expect } from "vitest"
import { fmtDateTime } from "./time"

const pad = (n: number) => String(n).padStart(2, "0")

describe("fmtDateTime", () => {
  it("renders a timestamp dated today as HH:MM only", () => {
    const d = new Date()
    d.setHours(9, 5, 0, 0)
    const out = fmtDateTime(d.toISOString())
    expect(out).toBe(`${pad(d.getHours())}:${pad(d.getMinutes())}`)
  })

  it("renders a same-year, non-today timestamp as MM-DD HH:MM", () => {
    const now = new Date()
    // Pick a day this year that is definitely not today: Jan 1 unless today is
    // Jan 1, in which case use Dec 31 (still the current year).
    const d = new Date(now)
    if (now.getMonth() === 0 && now.getDate() === 1) d.setMonth(11, 31)
    else d.setMonth(0, 1)
    d.setHours(14, 3, 0, 0)
    const out = fmtDateTime(d.toISOString())
    expect(out).toBe(
      `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`,
    )
  })

  it("renders a different-year timestamp with the year (YYYY-MM-DD HH:MM)", () => {
    const d = new Date()
    d.setFullYear(d.getFullYear() - 1)
    d.setMonth(2, 7) // March 7
    d.setHours(14, 3, 0, 0)
    const out = fmtDateTime(d.toISOString())
    expect(out).toBe(
      `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`,
    )
  })

  it("returns junk input unchanged", () => {
    expect(fmtDateTime("not a date")).toBe("not a date")
  })
})
