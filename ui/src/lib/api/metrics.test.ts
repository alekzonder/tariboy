import { afterEach, describe, expect, it, vi } from "vitest";
import { createApi } from "@/lib/api";
import { metricsReport } from "@/test/fixtures/apiFixtures";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function stubFetch(result: unknown) {
  const calls: Array<{ url: string; signal?: AbortSignal | null }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init: RequestInit) => {
      calls.push({ url, signal: init.signal });
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
  return calls;
}

describe("metrics api", () => {
  it("reads the server report with the period and only the filters given", async () => {
    const calls = stubFetch(metricsReport());
    const api = createApi({ id: "d1", label: "x", baseURL: "https://m.example/", token: "t" });

    const report = await api.metrics.getMetrics({
      since: "2026-10-03T14:05:00.000Z",
      until: "2026-11-02T14:05:00.000Z",
      image: "localhost:5000/dev:1.0",
    });
    await api.metrics.getMetrics({ since: "a", until: "b", agent: "alice", workflow: "none" });

    expect(report.total.tasks.tasks_done).toBe(7);
    expect(calls.map((c) => c.url)).toEqual([
      "https://m.example/api/metrics?since=2026-10-03T14%3A05%3A00.000Z&until=2026-11-02T14%3A05%3A00.000Z&image=localhost%3A5000%2Fdev%3A1.0",
      "https://m.example/api/metrics?since=a&until=b&agent=alice&workflow=none",
    ]);
  });

  it("passes the caller's signal, so a newer request can cancel an older one", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockImplementation(
          (_url: string, init: RequestInit) =>
            new Promise((_resolve, reject) =>
              init.signal?.addEventListener("abort", () => reject(new Error("aborted"))),
            ),
        ),
    );
    const controller = new AbortController();
    const pending = createApi(null).metrics.getMetrics(
      { since: "a", until: "b" },
      controller.signal,
    );
    controller.abort();
    await expect(pending).rejects.toThrow("aborted");
  });

  it("reads one task's metrics by its key", async () => {
    const calls = stubFetch({});
    await createApi(null).metrics.getTaskMetrics("DEV-1/x");
    expect(calls[0].url).toBe("/api/metrics/tasks/DEV-1%2Fx");
  });
});
