import { describe, it, expect } from "vitest";
import { descriptor, type AuditEvent } from "./audit";

const ev = (over: Partial<AuditEvent>): AuditEvent => ({
  seq: 1,
  kind: "iteration_started",
  source: "system",
  at: "t",
  data: "{}",
  iteration_id: "it-1",
  ...over,
});
const ho = (inner: Record<string, unknown>): string =>
  JSON.stringify({ line: JSON.stringify(inner) });
const asst = (block: Record<string, unknown>): string =>
  ho({ type: "assistant", message: { content: [block] } });

describe("descriptor dispatch tables", () => {
  it("assistant thinking or empty content → 🧠 thinking", () => {
    expect(descriptor(ev({ kind: "harness_output", data: asst({ type: "thinking" }) }))).toEqual({
      icon: "🧠",
      label: "thinking",
      preview: "",
      tone: "neutral",
    });
    expect(
      descriptor(ev({ kind: "harness_output", data: ho({ type: "assistant", message: {} }) }))
        .label,
    ).toBe("thinking");
  });

  it("unknown harness line types, including prototype names, use the generic row", () => {
    for (const type of ["stream_event", "constructor"]) {
      const d = descriptor(ev({ kind: "harness_output", data: ho({ type, x: 1 }) }));
      expect(d).toEqual({
        icon: "·",
        label: type,
        preview: JSON.stringify({ type, x: 1 }),
        tone: "neutral",
      });
    }
    expect(descriptor(ev({ kind: "harness_output", data: ho({}) })).label).toBe("harness");
  });

  it("iteration_done shares the iteration_finished row", () => {
    expect(
      descriptor(ev({ kind: "iteration_done", data: JSON.stringify({ status: "done" }) })),
    ).toEqual({ icon: "⏹", label: "iteration", preview: "done", tone: "success" });
  });

  it("launching_harness interactive, harness_spawned and shim lines", () => {
    expect(
      descriptor(
        ev({
          kind: "launching_harness",
          data: JSON.stringify({ harness: "codex", interactive: true }),
        }),
      ).preview,
    ).toBe("codex (interactive)");
    expect(descriptor(ev({ kind: "harness_spawned" }))).toEqual({
      icon: "🚀",
      label: "harness",
      preview: "spawned",
      tone: "neutral",
    });
    expect(descriptor(ev({ kind: "shim", data: JSON.stringify({ line: "ready" }) }))).toEqual({
      icon: "·",
      label: "shim",
      preview: "ready",
      tone: "neutral",
    });
  });

  it("unknown kinds, including prototype names, use the generic row", () => {
    expect(descriptor(ev({ kind: "budget_hit", data: JSON.stringify({ reason: "cap" }) }))).toEqual(
      { icon: "·", label: "budget_hit", preview: "cap", tone: "warn" },
    );
    expect(descriptor(ev({ kind: "toString", data: JSON.stringify({ n: 1 }) }))).toEqual({
      icon: "·",
      label: "toString",
      preview: JSON.stringify({ n: 1 }),
      tone: "neutral",
    });
  });
});
