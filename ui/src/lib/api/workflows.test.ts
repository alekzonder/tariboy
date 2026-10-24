import { afterEach, expect, it, vi } from "vitest";
import type { Daemon } from "../daemons";
import { ApiError, createApi } from ".";

afterEach(() => vi.restoreAllMocks());

it("exports and imports a workflow archive on explicit daemons", async () => {
  const target: Daemon = {
    id: "remote",
    label: "Remote",
    baseURL: "https://remote.example",
    token: "secret",
  };
  const calls: Array<{ url: string; init: RequestInit }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, init });
      if (init.method === "GET")
        return { ok: true, status: 200, blob: async () => new Blob(["archive"]) } as Response;
      return {
        ok: true,
        status: 200,
        json: async () => ({
          ok: true,
          result: {
            name: "dev",
            version: "1.0.0",
            digest: "d",
            tags: ["1.0.0", "latest"],
            created: true,
          },
        }),
      } as Response;
    }),
  );
  const { workflows } = createApi(target);
  const archive = await workflows.downloadWorkflowArchive("dev", "1.0.0");
  expect(await workflows.importWorkflowArchive(archive)).toEqual({
    name: "dev",
    version: "1.0.0",
    digest: "d",
    tags: ["1.0.0", "latest"],
    created: true,
  });
  expect(calls.map((call) => [call.init.method, call.url])).toEqual([
    ["GET", "https://remote.example/api/workflow-images/dev/1.0.0/export"],
    ["POST", "https://remote.example/api/workflow-image-imports"],
  ]);
  expect(calls[1].init.body).toBe(archive);
});

it("surfaces a refused workflow import as an ApiError", async () => {
  const target: Daemon = {
    id: "remote",
    label: "Remote",
    baseURL: "https://remote.example",
    token: "secret",
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        ({
          ok: false,
          status: 409,
          text: async () =>
            JSON.stringify({
              ok: false,
              error: { code: "workflow_version_published", message: "taken" },
            }),
        }) as Response,
    ),
  );
  const { workflows } = createApi(target);
  await expect(workflows.importWorkflowArchive(new Blob(["x"]))).rejects.toMatchObject({
    code: "workflow_version_published",
    message: "taken",
  });
  await expect(workflows.importWorkflowArchive(new Blob(["x"]))).rejects.toBeInstanceOf(ApiError);
});
