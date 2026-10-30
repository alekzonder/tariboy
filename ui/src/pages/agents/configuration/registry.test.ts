import { describe, expect, it } from "vitest";
import { SECTIONS, fieldOfKey, normalizeField, sectionOfKey, validateField } from "./registry";

const keysOf = (id: string) => SECTIONS.find((s) => s.id === id)!.fields.map((f) => f.key);

// Every setting the page had before the Settings layout; none may be lost.
const ALL_KEYS = [
  "image",
  "image-digest",
  "cwd",
  "cwd-create",
  "harness",
  "model",
  "effort",
  "interactive",
  "interval",
  "schedule",
  "timeout",
  "hard-timeout",
  "on-timeout",
  "on-error",
  "max-idle",
  "ai-stall-timeout",
  "goal-enabled",
  "goal-wait-customer-timeout",
  "goal-delivery-cooldown",
  "current-goal-task",
  "messages-batch",
  "messages-max-queue",
  "budget-hour",
  "budget-day",
  "budget-week",
  "budget-month",
  "shell-script",
  "secrets-remove",
  "secret-key",
  "secret-value",
  "keep-iterations",
  "keep-days",
];

describe("SECTIONS", () => {
  it("keeps every setting, each in exactly one section", () => {
    const keys = SECTIONS.flatMap((s) => s.fields.map((f) => f.key));
    expect(keys).toHaveLength(ALL_KEYS.length);
    expect(new Set(keys)).toEqual(new Set(ALL_KEYS));
  });

  it("puts the four groups first and everything else under Advanced", () => {
    expect(SECTIONS.map((s) => [s.id, s.advanced])).toEqual([
      ["model", false],
      ["folder", false],
      ["autopilot", false],
      ["spending", false],
      ["image", true],
      ["schedule", true],
      ["goal", true],
      ["mail", true],
      ["budgets", true],
      ["environment", true],
      ["retention", true],
    ]);
    expect(keysOf("model")).toEqual(["harness", "model", "effort"]);
    expect(keysOf("folder")).toEqual(["cwd", "cwd-create"]);
    expect(keysOf("autopilot")).toEqual(["interval", "timeout", "interactive"]);
    expect(keysOf("spending")).toEqual(["budget-day", "budget-month"]);
    expect(keysOf("budgets")).toEqual(["budget-hour", "budget-week"]);
  });

  it("names the groups and fields in words", () => {
    expect(SECTIONS.slice(0, 4).map((s) => s.title)).toEqual([
      "Model",
      "Working folder",
      "Autopilot",
      "Spending limit",
    ]);
    expect(fieldOfKey("harness")?.label).toBe("Coding tool");
    expect(fieldOfKey("interactive")?.label).toBe("Interactive terminal");
  });

  it("maps a field key to its section", () => {
    expect(sectionOfKey("model")?.id).toBe("model");
    expect(sectionOfKey("hard-timeout")?.id).toBe("schedule");
    expect(sectionOfKey("keep-days")?.id).toBe("retention");
    expect(sectionOfKey("nope")).toBeUndefined();
  });
});

describe("field helpers", () => {
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

  it("rejects an interval or timeout the minutes box could not convert", () => {
    for (const bad of ["", "abc", "-1"]) {
      expect(validateField("interval", bad, {})).toBe("Enter the minutes between runs.");
      expect(validateField("timeout", bad, {})).toBe("Enter the minutes, or 0 for no limit.");
    }
    expect(validateField("interval", "0", {})).toBe("");
    expect(validateField("interval", "300", {})).toBe("");
    expect(validateField("timeout", "0", {})).toBe("");
  });

  it("requires a secret key and a value together", () => {
    expect(validateField("secret-key", "", { "secret-value": "v" })).toMatch(/key/i);
    expect(validateField("secret-value", "", { "secret-key": "TOKEN" })).toMatch(/value/i);
    expect(validateField("secret-key", "TOKEN", { "secret-value": "v" })).toBe("");
  });
});
