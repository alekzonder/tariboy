import { describe, expect, it } from "vitest";
import { inputAttributes } from "./draftFieldModel";
import type { ConfigField } from "./registry";

const field = (fields: Partial<ConfigField>) =>
  ({ key: "x", label: "X", helper: "", kind: "text", ...fields }) as ConfigField;

describe("inputAttributes", () => {
  it("leaves a text input plain", () => {
    expect(inputAttributes(field({}))).toEqual({
      type: undefined,
      autoComplete: undefined,
      min: undefined,
      step: undefined,
    });
  });

  it("makes a number input with its minimum, zero by default", () => {
    expect(inputAttributes(field({ kind: "number" }))).toMatchObject({ type: "number", min: 0 });
    expect(inputAttributes(field({ kind: "number", minimum: 1 }))).toMatchObject({ min: 1 });
  });

  it("hides a secret value from autofill", () => {
    expect(inputAttributes(field({ kind: "secret-value" }))).toMatchObject({
      type: "password",
      autoComplete: "new-password",
      min: undefined,
    });
  });

  it("steps budgets by cents", () => {
    expect(inputAttributes(field({ key: "budget-day", kind: "number" }))).toMatchObject({
      step: "0.01",
    });
  });
});
