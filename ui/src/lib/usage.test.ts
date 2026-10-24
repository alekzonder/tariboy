import { describe, expect, it } from "vitest";
import { fmtCost } from "./usage";

describe("fmtCost", () => {
  it("formats zero with four decimal places", () => {
    expect(fmtCost(0)).toBe("$0.0000");
  });

  it("rounds to four decimal places", () => {
    expect(fmtCost(0.35)).toBe("$0.3500");
    expect(fmtCost(1.23456)).toBe("$1.2346");
    expect(fmtCost(0.00004)).toBe("$0.0000");
  });
});
