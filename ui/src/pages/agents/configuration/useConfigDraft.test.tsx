import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { Values } from "./registry";
import { useConfigDraft, type CommitUnit } from "./useConfigDraft";

// A tiny server: units write into it, and reload copies it into the baseline
// the hook sees, the way the page reloads its sources after a save.
function setup(initial: Values, fail: (unit: string) => boolean = () => false) {
  const server: Values = { ...initial };
  const sent: Array<{ unit: string; values: Values }> = [];
  const unit = (id: string, keys: string[]): CommitUnit => ({
    id,
    keys,
    submit: async (values) => {
      sent.push({ unit: id, values: Object.fromEntries(keys.map((k) => [k, values[k]])) });
      if (fail(id)) throw new Error(`${id} rejected`);
      for (const k of keys) server[k] = values[k];
    },
  });
  const units = [
    unit("interval", ["interval"]),
    unit("timeout", ["timeout"]),
    unit("budget", ["budget-hour", "budget-day"]),
    unit("goal", ["goal-delivery-cooldown"]),
  ];
  const onFocus = vi.fn();
  const hook = renderHook(
    ({ baseline }: { baseline: Values }) =>
      useConfigDraft({ baseline, units, reload, onFocus }),
    { initialProps: { baseline: { ...server } } },
  );
  async function reload() {
    hook.rerender({ baseline: { ...server } });
  }
  return { hook, sent, server, onFocus };
}

const base = { interval: "30", timeout: "60", "budget-hour": "1", "budget-day": "0", "goal-delivery-cooldown": "60" };

describe("useConfigDraft", () => {
  it("is dirty only while a normalized draft differs from the baseline", () => {
    const { hook } = setup(base);
    act(() => hook.result.current.setField("interval", " 30 "));
    expect(hook.result.current.dirtyKeys).toEqual([]);
    act(() => hook.result.current.setField("interval", "45"));
    expect(hook.result.current.dirtyKeys).toEqual(["interval"]);
    expect(hook.result.current.values.interval).toBe("45");
    act(() => hook.result.current.setField("interval", "30"));
    expect(hook.result.current.dirtyKeys).toEqual([]);
  });

  it("saves every changed unit with one call, in unit order, and only those", async () => {
    const { hook, sent } = setup(base);
    act(() => hook.result.current.setField("budget-day", "5"));
    act(() => hook.result.current.setField("interval", "45"));
    await act(() => hook.result.current.save());
    expect(sent).toEqual([
      { unit: "interval", values: { interval: "45" } },
      { unit: "budget", values: { "budget-hour": "1", "budget-day": "5" } },
    ]);
    expect(hook.result.current.dirtyKeys).toEqual([]);
    expect(hook.result.current.notice).toBe("All changes saved");
  });

  it("sends nothing when a changed field is invalid and focuses it", async () => {
    const { hook, sent, onFocus } = setup(base);
    act(() => hook.result.current.setField("interval", "45"));
    act(() => hook.result.current.setField("goal-delivery-cooldown", "0"));
    await act(() => hook.result.current.save());
    expect(sent).toEqual([]);
    expect(hook.result.current.fieldErrors["goal-delivery-cooldown"]).toMatch(/positive whole number/);
    expect(onFocus).toHaveBeenCalledWith("goal-delivery-cooldown");
    expect(hook.result.current.dirtyKeys).toEqual(["interval", "goal-delivery-cooldown"]);
  });

  it("stops at the first failure, keeps it and the unsent drafts, and drops the saved ones", async () => {
    const { hook, sent, onFocus, server } = setup(base, (unit) => unit === "timeout");
    act(() => hook.result.current.setField("interval", "45"));
    act(() => hook.result.current.setField("timeout", "90"));
    act(() => hook.result.current.setField("budget-hour", "3"));
    await act(() => hook.result.current.save());
    expect(sent.map((s) => s.unit)).toEqual(["interval", "timeout"]);
    expect(server.interval).toBe("45");
    expect(hook.result.current.dirtyKeys).toEqual(["timeout", "budget-hour"]);
    expect(hook.result.current.fieldErrors.timeout).toBe("timeout rejected");
    expect(hook.result.current.error).toMatch(/not saved/);
    expect(onFocus).toHaveBeenCalledWith("timeout");
  });

  it("retries only what is still dirty", async () => {
    let failing = true;
    const { hook, sent } = setup(base, (unit) => failing && unit === "timeout");
    act(() => hook.result.current.setField("interval", "45"));
    act(() => hook.result.current.setField("timeout", "90"));
    await act(() => hook.result.current.save());
    failing = false;
    sent.length = 0;
    await act(() => hook.result.current.save());
    expect(sent.map((s) => s.unit)).toEqual(["timeout"]);
    expect(hook.result.current.dirtyKeys).toEqual([]);
  });

  it("reset drops every draft without sending anything", () => {
    const { hook, sent } = setup(base);
    act(() => hook.result.current.setField("interval", "45"));
    act(() => hook.result.current.setField("budget-day", "9"));
    act(() => hook.result.current.reset());
    expect(hook.result.current.dirtyKeys).toEqual([]);
    expect(hook.result.current.values.interval).toBe("30");
    expect(sent).toEqual([]);
  });

  it("keeps drafts through a reload that changes other baseline values", () => {
    const { hook } = setup(base);
    act(() => hook.result.current.setField("interval", "45"));
    hook.rerender({ baseline: { ...base, timeout: "61" } });
    expect(hook.result.current.values.interval).toBe("45");
    expect(hook.result.current.values.timeout).toBe("61");
  });

  it("ignores a field whose source has not loaded", () => {
    const { hook } = setup(base);
    act(() => hook.result.current.setField("keep-days", "5"));
    expect(hook.result.current.dirtyKeys).toEqual([]);
  });
});
