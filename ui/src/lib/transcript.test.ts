import { describe, it, expect, vi, afterEach } from "vitest";
import * as api from "@/lib/api";
import { fetchTranscript } from "@/lib/transcript";

afterEach(() => vi.restoreAllMocks());

describe("fetchTranscript", () => {
  it("returns the calls array", async () => {
    vi.spyOn(api, "agentGet").mockResolvedValue({
      calls: [{ seq: 0, ts: "t", provider: "anthropic", model: "m", instructions: "S1", instructions_changed: true, delta: [], response: { blocks: [] } }],
      count: 1,
    } as unknown as never);
    const calls = await fetchTranscript("a1", "it-1");
    expect(calls).toHaveLength(1);
    expect(calls[0].instructions).toBe("S1");
    expect(api.agentGet).toHaveBeenCalledWith("a1", "iterations/it-1/transcript");
  });

  it("returns [] on empty", async () => {
    vi.spyOn(api, "agentGet").mockResolvedValue({ calls: [], count: 0 } as unknown as never);
    expect(await fetchTranscript("a1", "it-1")).toEqual([]);
  });
});
