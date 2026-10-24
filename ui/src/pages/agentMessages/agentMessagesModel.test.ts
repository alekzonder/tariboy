import { describe, expect, it } from "vitest";
import type { InboxItem } from "@/lib/api";
import { dialogCopy, messageRowFlags } from "./agentMessagesModel";

const item: InboxItem = {
  id: "m1",
  channel: "agent.alice",
  ts: "2026-07-12T10:00:00Z",
  source: "bob",
  type: "note",
  text: "hi",
  attempts: 1,
  dlq: false,
};

describe("messageRowFlags", () => {
  it("hides every optional label on a plain event", () => {
    expect(messageRowFlags({ item: { ...item, kind: "event" }, view: "queue" })).toEqual({
      kind: "",
      showAttempts: false,
      showProcessed: false,
      hasDetails: false,
    });
  });

  it("shows the kind, retries and details when present", () => {
    expect(
      messageRowFlags({
        item: { ...item, kind: "request", attempts: 2, data: { a: 1 } },
        view: "dlq",
      }),
    ).toEqual({ kind: "request", showAttempts: true, showProcessed: false, hasDetails: true });
  });

  it("counts a subject alone as details", () => {
    expect(
      messageRowFlags({ item: { ...item, subject: { k: 1 } }, view: "queue" }).hasDetails,
    ).toBe(true);
  });

  it.each([
    ["archive", "2026-07-12T11:00:00Z", true],
    ["archive", undefined, false],
    ["queue", "2026-07-12T11:00:00Z", false],
  ] as const)("shows the processed line in %s with processed_at=%s: %s", (view, at, shown) => {
    expect(messageRowFlags({ item: { ...item, processed_at: at }, view }).showProcessed).toBe(
      shown,
    );
  });
});

describe("dialogCopy", () => {
  it.each([
    ["clear", "Clear queue", "destructive", ""],
    ["processed", "Mark processed", "default", "result is required"],
    ["reply", "Reply", "default", "reply text is required"],
  ] as const)("describes the %s dialog", (mode, title, variant, required) => {
    const copy = dialogCopy(mode);
    expect(copy.title).toBe(title);
    expect(copy.variant).toBe(variant);
    expect(copy.required).toBe(required);
  });

  it("falls back to the processed copy while closed", () => {
    expect(dialogCopy(undefined).title).toBe("Mark processed");
  });
});
