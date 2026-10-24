import { afterEach, describe, expect, it, vi } from "vitest";
import type { Daemon } from "../daemons";
import { createApi } from ".";

interface Call {
  url: string;
  method: string;
  body?: unknown;
  authorization?: string;
}

const target: Daemon = {
  id: "remote",
  label: "Remote",
  baseURL: "https://remote.example/",
  token: "secret",
};

function captureFetch(calls: Call[], results: unknown[] = []) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init: RequestInit) => {
      const headers = (init.headers ?? {}) as Record<string, string>;
      calls.push({
        url,
        method: init.method ?? "GET",
        body: init.body ? JSON.parse(init.body as string) : undefined,
        authorization: headers.Authorization,
      });
      const result = results.shift() ?? {};
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
}

afterEach(() => vi.restoreAllMocks());

describe("managed image source client", () => {
  it("targets one explicit host for CRUD, validation, and build", async () => {
    const calls: Call[] = [];
    captureFetch(calls);
    const {
      buildImageSource,
      createImageSource,
      deleteImageSource,
      getImageSource,
      getImageSourceFile,
      listImageSourceFiles,
      listImageSources,
      putImageSourceFile,
      validateImageSource,
    } = createApi(target).imageSources;

    await listImageSources();
    await createImageSource({
      name: "reviewer dev",
      prompt: "Review.",
    });
    await getImageSource("reviewer dev");
    await listImageSourceFiles("reviewer dev");
    await getImageSourceFile("reviewer dev", "skills/a b/#review.md");
    await putImageSourceFile("reviewer dev", "skills/a b/#review.md", "# Review");
    await validateImageSource("reviewer dev");
    await buildImageSource("reviewer dev", "canary");
    await deleteImageSource("reviewer dev");

    expect(calls.map(({ url, method }) => ({ url, method }))).toEqual([
      { url: "https://remote.example/api/image-sources", method: "GET" },
      { url: "https://remote.example/api/image-sources", method: "POST" },
      { url: "https://remote.example/api/image-sources/reviewer%20dev", method: "GET" },
      { url: "https://remote.example/api/image-sources/reviewer%20dev/files", method: "GET" },
      {
        url: "https://remote.example/api/image-sources/reviewer%20dev/files/skills/a%20b/%23review.md",
        method: "GET",
      },
      {
        url: "https://remote.example/api/image-sources/reviewer%20dev/files/skills/a%20b/%23review.md",
        method: "PUT",
      },
      { url: "https://remote.example/api/image-sources/reviewer%20dev/validate", method: "POST" },
      { url: "https://remote.example/api/image-sources/reviewer%20dev/build", method: "POST" },
      { url: "https://remote.example/api/image-sources/reviewer%20dev", method: "DELETE" },
    ]);
    expect(calls.every((call) => call.authorization === "Bearer secret")).toBe(true);
    expect(calls[1].body).toMatchObject({
      name: "reviewer dev",
      prompt: "Review.",
    });
    expect(calls[5].body).toEqual({ content: "# Review" });
    expect(calls[7].body).toEqual({ tag: "canary" });
  });

  it("preserves structured validation diagnostics", async () => {
    const calls: Call[] = [];
    captureFetch(calls, [
      {
        valid: false,
        diagnostics: [
          { path: "Tariboyfile.yaml", field: "harness.type", message: "unsupported harness" },
        ],
      },
    ]);

    const result = await createApi(null).imageSources.validateImageSource("reviewer");

    expect(result).toEqual({
      valid: false,
      diagnostics: [
        { path: "Tariboyfile.yaml", field: "harness.type", message: "unsupported harness" },
      ],
    });
    expect(calls[0].url).toBe("/api/image-sources/reviewer/validate");
  });
});
