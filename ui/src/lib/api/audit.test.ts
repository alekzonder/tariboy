import { describe, it, expect, vi, afterEach } from "vitest";
import { createApi } from "@/lib/api";

function stubResult(result: unknown) {
  const fetchMock = vi.fn().mockImplementation(() =>
    Promise.resolve({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ ok: true, result }),
    } as Response),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("fetchTranscript", () => {
  it("returns the calls array", async () => {
    const fetchMock = stubResult({
      calls: [
        {
          seq: 0,
          ts: "t",
          provider: "anthropic",
          model: "m",
          instructions: "S1",
          instructions_changed: true,
          delta: [],
          response: { blocks: [] },
        },
      ],
      count: 1,
    });
    const calls = await createApi(null).audit.fetchTranscript("a1", "it-1");
    expect(calls).toHaveLength(1);
    expect(calls[0].instructions).toBe("S1");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/agents/a1/iterations/it-1/transcript",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("returns [] on empty", async () => {
    stubResult({ calls: [], count: 0 });
    expect(await createApi(null).audit.fetchTranscript("a1", "it-1")).toEqual([]);
  });

  it("passes the cancellation signal to the request", async () => {
    const fetchMock = stubResult({ calls: [], count: 0 });
    const controller = new AbortController();
    await createApi(null).audit.fetchTranscript("a1", "it-1", controller.signal);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/agents/a1/iterations/it-1/transcript",
      expect.objectContaining({ signal: controller.signal }),
    );
  });
});
