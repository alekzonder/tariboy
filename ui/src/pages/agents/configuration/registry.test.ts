import { describe, expect, it } from "vitest";
import { BLOCKS, filterBlocks, blockOfKey, normalizeField, validateField } from "./registry";

const ids = (query: string) => filterBlocks(query, BLOCKS).blocks.map((b) => b.id);

describe("filterBlocks", () => {
  it("shows every block for an empty query", () => {
    expect(ids("")).toEqual(BLOCKS.map((b) => b.id));
    expect(ids("   ")).toHaveLength(BLOCKS.length);
  });

  it("shows the whole block when one setting matches and marks only that setting", () => {
    const result = filterBlocks("hard timeout", BLOCKS);
    expect(result.blocks.map((b) => b.id)).toEqual(["loop"]);
    expect(result.blocks[0].fields.length).toBeGreaterThan(1);
    expect(result.matched.has("hard-timeout")).toBe(true);
    expect(result.matched.has("interval")).toBe(false);
    expect(result.matched.has("timeout")).toBe(false);
  });

  it("matches case-insensitively by block title", () => {
    expect(ids("BUDGET")).toEqual(["budgets"]);
  });

  it("matches keywords that are not visible labels", () => {
    expect(ids("cwd")).toEqual(["general"]);
    expect(ids("tmux")).toEqual(["runtime"]);
    expect(ids("dlq")).toEqual(["messages"]);
    expect(ids("usd")).toEqual(["budgets"]);
  });

  it("requires every word to match inside one block", () => {
    expect(ids("secret key")).toEqual(["environment"]);
    expect(ids("budget tmux")).toEqual([]);
  });

  it("returns no blocks when nothing matches", () => {
    const result = filterBlocks("zzz-nothing", BLOCKS);
    expect(result.blocks).toEqual([]);
    expect(result.matched.size).toBe(0);
  });
});

describe("field helpers", () => {
  it("maps a field key to its block", () => {
    expect(blockOfKey("model")).toBe("runtime");
    expect(blockOfKey("budget-day")).toBe("budgets");
    expect(blockOfKey("keep-days")).toBe("retention");
  });

  it("normalizes whole numbers and trims text", () => {
    expect(normalizeField("interval", " 30.7 ")).toBe("30");
    expect(normalizeField("max-idle", "-4")).toBe("0");
    expect(normalizeField("interval", "abc")).toBe("abc");
    expect(normalizeField("cwd", "  /srv/new ")).toBe("/srv/new");
    expect(normalizeField("shell-script", "echo hi\n")).toBe("echo hi\n");
    expect(normalizeField("secret-value", " keep spaces ")).toBe(" keep spaces ");
  });

  it("validates positive Goal and message numbers and budget amounts", () => {
    expect(validateField("goal-delivery-cooldown", "0", {})).toMatch(/positive whole number/);
    expect(validateField("messages-batch", "5", {})).toBe("");
    expect(validateField("budget-hour", "-1", {})).toMatch(/zero or a positive/);
    expect(validateField("budget-hour", "2.5", {})).toBe("");
    expect(validateField("keep-days", "1.5", {})).toMatch(/whole number/);
  });

  it("requires a secret key and a value together", () => {
    expect(validateField("secret-key", "", { "secret-value": "v" })).toMatch(/key/i);
    expect(validateField("secret-value", "", { "secret-key": "TOKEN" })).toMatch(/value/i);
    expect(validateField("secret-key", "TOKEN", { "secret-value": "v" })).toBe("");
  });
});
