import { describe, it, expect, vi, afterEach } from "vitest";
import { createApi } from "@/lib/api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("built-image reads", () => {
  it("addresses every built-image read through the client's host", async () => {
    const calls: Array<{ url: string; method: string }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((url: string, init: RequestInit) => {
        calls.push({ url, method: init.method ?? "GET" });
        return Promise.resolve({
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ ok: true, result: {} }),
        } as Response);
      }),
    );
    const d = { id: "d1", label: "x", baseURL: "https://images.example/", token: "t" };
    const api = createApi(d);

    await api.images.listImages();
    await api.images.imageManifestGet("reviewer:latest");
    await api.images.imagePromptGet("reviewer:latest");
    await api.images.imageFilesList("reviewer:latest");
    await api.images.imageFileRead("reviewer:latest", "skills/a b/#review.md");

    expect(calls).toEqual([
      { url: "https://images.example/api/images", method: "GET" },
      { url: "https://images.example/api/images/reviewer%3Alatest", method: "GET" },
      { url: "https://images.example/api/images/reviewer%3Alatest/prompt", method: "GET" },
      { url: "https://images.example/api/images/reviewer%3Alatest/files", method: "GET" },
      {
        url: "https://images.example/api/images/reviewer%3Alatest/files/skills/a%20b/%23review.md",
        method: "GET",
      },
    ]);
  });
});
