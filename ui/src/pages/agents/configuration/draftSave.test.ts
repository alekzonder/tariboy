import { describe, expect, it } from "vitest";
import type { Values } from "./registry";
import { releaseSent, submitUnits, validateDraft } from "./draftSave";
import type { CommitUnit } from "./useConfigDraft";

describe("validateDraft", () => {
  const units: CommitUnit[] = [
    { id: "batch", keys: ["messages-batch"], submit: async () => {} },
    { id: "secret", keys: ["secret-key", "secret-value"], submit: async () => {} },
  ];

  it("returns no errors for valid dirty fields", () => {
    expect(
      validateDraft({
        dirtyKeys: ["messages-batch"],
        units,
        baseline: { "messages-batch": "1" },
        normalized: { "messages-batch": "2" },
      }),
    ).toEqual({});
  });

  it("reports an invalid dirty field", () => {
    expect(
      validateDraft({
        dirtyKeys: ["messages-batch"],
        units,
        baseline: { "messages-batch": "1" },
        normalized: { "messages-batch": "0" },
      }),
    ).toEqual({ "messages-batch": "Enter a positive whole number." });
  });

  it("checks an unchanged partner of a dirty pair", () => {
    expect(
      validateDraft({
        dirtyKeys: ["secret-value"],
        units,
        baseline: { "secret-key": "", "secret-value": "" },
        normalized: { "secret-key": "", "secret-value": "s3cret" },
      }),
    ).toEqual({ "secret-key": "Enter a secret key." });
  });

  it("skips a partner whose source has not loaded", () => {
    expect(
      validateDraft({
        dirtyKeys: ["secret-value"],
        units,
        baseline: { "secret-value": "" },
        normalized: { "secret-key": "", "secret-value": "s3cret" },
      }),
    ).toEqual({});
  });
});

describe("submitUnits", () => {
  const recorder = (fail = "") => {
    const calls: string[] = [];
    const unit = (id: string, keys: string[]): CommitUnit => ({
      id,
      keys,
      submit: async (values: Values) => {
        calls.push(id);
        expect(values).toBe(normalized);
        if (id === fail) throw new Error(`${id} rejected`);
      },
    });
    return {
      calls,
      units: [unit("a", ["a"]), unit("b", ["b1", "b2"]), unit("c", ["c"]), unit("d", ["d"])],
    };
  };
  const normalized: Values = { a: "1", b1: "2", b2: "3", c: "4", d: "5" };
  const drafts: Values = { a: " 1", b2: "3", c: "4", d: "5" };

  it("sends only changed units in order and collects their drafts", async () => {
    const { calls, units } = recorder();
    const result = await submitUnits({ units, dirtyKeys: ["a", "b2", "d"], drafts, normalized });
    expect(calls).toEqual(["a", "b", "d"]);
    expect(result).toEqual({
      sent: { a: " 1", b2: "3", d: "5" },
      failedKey: "",
      failedMessage: "",
    });
  });

  it("stops at the first failure and names its first changed key", async () => {
    const { calls, units } = recorder("b");
    const result = await submitUnits({ units, dirtyKeys: ["a", "b2", "d"], drafts, normalized });
    expect(calls).toEqual(["a", "b"]);
    expect(result).toEqual({ sent: { a: " 1" }, failedKey: "b2", failedMessage: "b rejected" });
  });

  it("reads a non-Error rejection as text", async () => {
    const units: CommitUnit[] = [
      {
        id: "a",
        keys: ["a"],
        submit: () => Promise.reject("nope"),
      },
    ];
    const result = await submitUnits({ units, dirtyKeys: ["a"], drafts, normalized });
    expect(result).toEqual({ sent: {}, failedKey: "a", failedMessage: "nope" });
  });
});

describe("releaseSent", () => {
  it("drops acknowledged drafts but keeps ones edited again during the save", () => {
    expect(releaseSent({ a: "1", b: "new", c: "3" }, { a: "1", b: "old" })).toEqual({
      b: "new",
      c: "3",
    });
  });
});
