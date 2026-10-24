import type { PropsWithChildren } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiContext } from "@/components/apiContext";
import type { ApiClient } from "@/lib/api";
import { useAuditFilter } from "./useAuditFilter";

const ev = (seq: number) => ({ seq, kind: "status", source: "system", at: `t${seq}`, data: "{}" });

function setup(result: { events: ReturnType<typeof ev>[]; capped?: boolean }) {
  const listAuditEvents = vi.fn(async () => result);
  const listAuditEventTypes = vi.fn(async () => ({ types: ["status", "harness_output"] }));
  const api = { audit: { listAuditEvents, listAuditEventTypes } } as unknown as ApiClient;
  const wrapper = ({ children }: PropsWithChildren) => (
    <ApiContext.Provider value={api}>{children}</ApiContext.Provider>
  );
  const hook = renderHook(() => useAuditFilter("dev-worker"), { wrapper });
  return { ...hook, listAuditEvents };
}

afterEach(() => vi.useRealTimers());

describe("useAuditFilter", () => {
  it("loads the type presets", async () => {
    const { result } = setup({ events: [] });
    await waitFor(() => expect(result.current.presets).toEqual(["status", "harness_output"]));
    expect(result.current.filtering).toBe(false);
  });

  it("debounces the query and returns matches chronological with capped", async () => {
    vi.useFakeTimers();
    const { result, listAuditEvents } = setup({ events: [ev(2), ev(1)], capped: true });
    act(() => result.current.setFilter(" status ", "boo"));
    act(() => result.current.setFilter(" status ", "boom"));
    expect(result.current.filtering).toBe(true);
    expect(result.current.filtered).toBeNull();
    expect(listAuditEvents).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(listAuditEvents).toHaveBeenCalledTimes(1);
    expect(listAuditEvents).toHaveBeenCalledWith("dev-worker", "type=status&q=boom");
    expect(result.current.filtered?.map((e) => e.seq)).toEqual([1, 2]);
    expect(result.current.capped).toBe(true);
  });

  it("drops matches and capped when the filter is cleared", async () => {
    vi.useFakeTimers();
    const { result } = setup({ events: [ev(1)], capped: true });
    act(() => result.current.setFilter("", "boom"));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(result.current.filtered).toHaveLength(1);
    act(() => result.current.setFilter("", ""));
    expect(result.current.filtering).toBe(false);
    expect(result.current.filtered).toBeNull();
    expect(result.current.capped).toBe(false);
  });
});
