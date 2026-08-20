import { describe, it, expect } from "vitest"
import { HOTKEY_BYTES } from "@/lib/terminalBytes"

describe("HOTKEY_BYTES", () => {
  it("maps each hotkey to its raw terminal byte sequence", () => {
    expect(HOTKEY_BYTES.esc).toBe("\x1b")
    expect(HOTKEY_BYTES.enter).toBe("\r")
    expect(HOTKEY_BYTES.ctrlc).toBe("\x03")
    expect(HOTKEY_BYTES.up).toBe("\x1b[A")
    expect(HOTKEY_BYTES.down).toBe("\x1b[B")
  })
})
