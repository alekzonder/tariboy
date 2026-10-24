import { afterEach, describe, expect, it, vi } from "vitest";
import { idempotencyKey } from "./ids";

describe("idempotencyKey", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("uses a random UUID when the WebView provides one", () => {
    vi.stubGlobal("crypto", { randomUUID: () => "uuid-1" });
    expect(idempotencyKey("comment")).toBe("uuid-1");
  });

  it("falls back to a prefixed time and random key without randomUUID", () => {
    vi.stubGlobal("crypto", {});
    expect(idempotencyKey("pool")).toMatch(/^pool-\d+-.+$/);
  });
});
