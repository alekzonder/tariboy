import { describe, expect, it } from "vitest";
import { commaFieldError, serializeEnv, type EnvRow } from "./AgentFormFields";

// parseKV mirrors the daemon's env parser (internal/commands/agents.go): split
// the K=V,K=V string on ',' with no escaping, then split each pair on the first
// '='. Used here to prove that a comma-bearing value would round-trip WRONG on
// the daemon side — which is exactly why commaFieldError rejects it up front.
function parseKV(list: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const pair of list.split(",")) {
    const p = pair.trim();
    if (p === "") continue;
    const i = p.indexOf("=");
    if (i >= 0) out[p.slice(0, i)] = p.slice(i + 1);
  }
  return out;
}

describe("serializeEnv", () => {
  it("joins rows as K=V,K=V and drops blank keys", () => {
    const rows: EnvRow[] = [
      { key: "A", value: "1" },
      { key: "  ", value: "ignored" },
      { key: " B ", value: "2" },
    ];
    expect(serializeEnv(rows)).toBe("A=1,B=2");
  });

  it("round-trips a comma-free value through the daemon parser", () => {
    const s = serializeEnv([{ key: "TOKEN", value: "abc123" }]);
    expect(parseKV(s)).toEqual({ TOKEN: "abc123" });
  });

  it("a comma-bearing value is mis-split by the daemon parser (the bug this guards)", () => {
    // serializeEnv itself has no way to represent the comma, so the daemon
    // splits "a,b" into a bogus extra pair and loses the real value.
    const s = serializeEnv([{ key: "LIST", value: "a,b" }]);
    expect(parseKV(s)).not.toEqual({ LIST: "a,b" });
  });
});

describe("commaFieldError", () => {
  it("returns null when no env value, env key, or plugin contains a comma", () => {
    const rows: EnvRow[] = [
      { key: "A", value: "1" },
      { key: "B", value: "two words" },
      { key: "", value: "a,b" }, // blank key row is dropped, so its comma is harmless
    ];
    expect(commaFieldError(rows, ["review-provider", "messenger"])).toBeNull();
  });

  it("rejects a comma in an env value with a clear message", () => {
    const rows: EnvRow[] = [{ key: "LIST", value: "a,b" }];
    const err = commaFieldError(rows, []);
    expect(err).toMatch(/LIST/);
    expect(err).toMatch(/comma/i);
  });

  it("rejects a comma in an env key", () => {
    expect(commaFieldError([{ key: "A,B", value: "1" }], [])).toMatch(/comma/i);
  });

  it("rejects a comma in a plugin name", () => {
    const err = commaFieldError([], ["ok", "a,b"]);
    expect(err).toMatch(/a,b/);
    expect(err).toMatch(/comma/i);
  });
});
