import { describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { ApiError, type MetricsFilter, type MetricsReport } from "@/lib/api";
import { apiWrapper, fakeApi } from "@/test/api";
import { metricsReport } from "@/test/fixtures/apiFixtures";
import { useMetrics } from "./useMetrics";

type Call = {
  filter: MetricsFilter;
  signal?: AbortSignal;
  resolve: (r: MetricsReport) => void;
  reject: (e: unknown) => void;
};

function setup(initial: MetricsFilter | null) {
  const calls: Call[] = [];
  const getMetrics = vi.fn(
    (filter: MetricsFilter, signal?: AbortSignal) =>
      new Promise<MetricsReport>((resolve, reject) =>
        calls.push({ filter, signal, resolve, reject }),
      ),
  );
  const client = fakeApi({ metrics: { getMetrics } });
  const hook = renderHook((props: { filter: MetricsFilter | null }) => useMetrics(props.filter), {
    initialProps: { filter: initial },
    wrapper: apiWrapper(client),
  });
  return { calls, getMetrics, ...hook };
}

const WEEK = { since: "2026-10-26T00:00:00.000Z", until: "2026-11-02T00:00:00.000Z" };
const MONTH = { since: "2026-10-03T00:00:00.000Z", until: "2026-11-02T00:00:00.000Z" };

describe("useMetrics", () => {
  it("makes one request per period and filter and shows its report", async () => {
    const { calls, result, rerender } = setup(WEEK);
    expect(result.current).toMatchObject({ report: null, loading: true, updating: false });
    rerender({ filter: { ...WEEK } });
    expect(calls).toHaveLength(1);
    await act(async () => calls[0].resolve(metricsReport()));
    expect(result.current.report?.total.tasks.tasks_done).toBe(7);
    expect(result.current.loading).toBe(false);
    expect(result.current.updatedAt).toBeInstanceOf(Date);
  });

  it("makes no request without a usable period", () => {
    const { calls, result } = setup(null);
    expect(calls).toHaveLength(0);
    expect(result.current.loading).toBe(false);
  });

  it("cancels the older request and never shows its late answer", async () => {
    const { calls, result, rerender } = setup(WEEK);
    rerender({ filter: { ...MONTH, agent: "alice" } });
    expect(calls).toHaveLength(2);
    expect(calls[0].signal?.aborted).toBe(true);
    await act(async () => calls[1].resolve(metricsReport({ since: "month" })));
    await act(async () => calls[0].resolve(metricsReport({ since: "week" })));
    expect(result.current.report?.since).toBe("month");
  });

  it("keeps the last report on screen, marked as updating, while a new one loads", async () => {
    const { calls, result, rerender } = setup(WEEK);
    await act(async () => calls[0].resolve(metricsReport({ since: "week" })));
    rerender({ filter: MONTH });
    expect(result.current).toMatchObject({ updating: true, loading: false });
    expect(result.current.report?.since).toBe("week");
    await act(async () => calls[1].resolve(metricsReport({ since: "month" })));
    expect(result.current.updating).toBe(false);
  });

  it("shows the error of the current request and loads again on retry", async () => {
    const { calls, result } = setup(WEEK);
    const failure = new ApiError(500, "internal", "boom");
    await act(async () => calls[0].reject(failure));
    expect(result.current.error).toBe(failure);
    act(() => result.current.retry());
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(result.current.loading).toBe(true);
    await act(async () => calls[1].resolve(metricsReport()));
    expect(result.current.error).toBeNull();
  });
});
