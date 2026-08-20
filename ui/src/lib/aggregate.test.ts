import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fetchAllAgents } from "./aggregate";
import { addDaemon } from "@/lib/daemons";

// Route the mocked fetch by the URL host so each daemon gets its own response +
// its own token assertion (the federation discriminant).
function routedFetch(routes: Record<string, { status: number; body: unknown }>, seen: Record<string, string>) {
  return vi.fn().mockImplementation((url: string, init: RequestInit) => {
    const key = Object.keys(routes).find((k) => url.startsWith(k)) ?? "";
    const auth = (init.headers as Record<string, string> | undefined)?.["Authorization"] ?? "";
    if (key) seen[key] = auth;
    const r = routes[key] ?? { status: 404, body: { ok: false, error: { code: "nf", message: "no route" } } };
    return Promise.resolve({
      ok: r.status >= 200 && r.status < 300,
      status: r.status,
      text: async () => JSON.stringify(r.body),
    } as unknown as Response);
  });
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => vi.restoreAllMocks());

describe("fetchAllAgents", () => {
  it("fans out to local + registered daemons, each with its OWN token, and merges", async () => {
    await addDaemon({ label: "prod", baseURL: "https://prod:8765", token: "tp" });
    await addDaemon({ label: "stage", baseURL: "https://stage:8765", token: "ts" });
    const seen: Record<string, string> = {};
    vi.stubGlobal(
      "fetch",
      routedFetch(
        {
          "/api/agents": { status: 200, body: { ok: true, result: { agents: [{ name: "local-a", image: "i", state: "running", harness: "claude", loop_enabled: true, group: null }], count: 1 } } },
          "/api/groups": { status: 200, body: { ok: true, result: { groups: [{ name: "empty-team", lead: "", members: 0 }], count: 1 } } },
          "https://prod:8765/api/agents": { status: 200, body: { ok: true, result: { agents: [{ name: "prod-a", image: "i", state: "running", harness: "claude", loop_enabled: false, group: null }], count: 1 } } },
          "https://prod:8765/api/groups": { status: 200, body: { ok: true, result: { groups: [], count: 0 } } },
          "https://stage:8765/api/agents": { status: 401, body: { ok: false, error: { code: "unauthorized", message: "nope" } } },
        },
        seen,
      ),
    );

    const out = await fetchAllAgents();
    const byLabel = Object.fromEntries(out.map((h) => [h.host.label, h]));
    expect(byLabel["This daemon (local)"].agents.map((a) => a.name)).toEqual(["local-a"]);
    expect(byLabel["prod"].agents.map((a) => a.name)).toEqual(["prod-a"]);
    expect(byLabel["This daemon (local)"].groups?.map((group) => group.name)).toEqual(["empty-team"]);
    // stage failed → degraded row, others intact.
    expect(byLabel["stage"].error).toBeTruthy();
    expect(byLabel["stage"].agents).toEqual([]);
    // Each cross-origin host was called with ITS token; local with none.
    expect(seen["https://prod:8765/api/agents"]).toBe("Bearer tp");
    expect(seen["https://stage:8765/api/agents"]).toBe("Bearer ts");
    expect(seen["/api/agents"]).toBe("");
  });
});
