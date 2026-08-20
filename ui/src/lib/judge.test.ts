import { describe, expect, it, vi, afterEach } from "vitest";
import { listJudgeRuns } from "./judge";

afterEach(() => vi.restoreAllMocks());

describe("judge API helpers", () => {
  it("lists operator judge runs", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      text: async () => JSON.stringify({ ok: true, result: { runs: [], count: 0 } }),
    } as Response);
    vi.stubGlobal("fetch", fetchMock);

    await expect(listJudgeRuns()).resolves.toEqual({ runs: [], count: 0 });
    expect(fetchMock).toHaveBeenCalledWith("/api/judges", expect.objectContaining({ method: "GET" }));
  });
});
