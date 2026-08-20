import { afterEach, expect, it, vi } from "vitest";
import type { Daemon } from "./daemons";
import { downloadTeamArchiveOn, getTeamOn, importTeamYAMLOn } from "./teamApi";

afterEach(() => vi.restoreAllMocks());

it("routes every team JSON call to the explicit daemon", async () => {
  const target: Daemon = { id: "remote", label: "Remote", baseURL: "https://remote.example", token: "secret" };
  const calls: Array<{ url: string; init: RequestInit }> = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    return { ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: { name: "team", lead: "lead", members: [] } }) } as Response;
  }));
  await getTeamOn(target, "team/name");
  await importTeamYAMLOn(target, "version: 1\n");
  expect(calls.map((call) => call.url)).toEqual([
    "https://remote.example/api/groups/team%2Fname",
    "https://remote.example/api/team-imports/preview-yaml",
  ]);
  for (const call of calls) expect((call.init.headers as Record<string, string>).Authorization).toBe("Bearer secret");
});

it("downloads a team archive from the explicit daemon", async () => {
  const target: Daemon = { id: "remote", label: "Remote", baseURL: "https://remote.example", token: "secret" };
  let seen = "";
  vi.stubGlobal("fetch", vi.fn(async (url: string) => {
    seen = url;
    return { ok: true, status: 200, headers: new Headers({"Content-Type":"application/gzip"}), text: async () => "", blob: async () => new Blob(["archive"]) } as Response;
  }));
  const archive = await downloadTeamArchiveOn(target, "dev/team");
  expect(seen).toBe("https://remote.example/api/groups/dev%2Fteam/export");
  expect(archive.size).toBe(7);
});
