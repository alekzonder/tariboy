import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import {
  storeApi, StoreApiError, setToken, clearToken, hasToken,
  getInfo, listRepos, getTags, getManifest, probeAuth,
} from "./storeApi";

function mockFetch(status: number, body: unknown, capture?: (init: RequestInit) => void) {
  return vi.fn().mockImplementation((_url: string, init: RequestInit) => {
    capture?.(init);
    return Promise.resolve({
      ok: status >= 200 && status < 300,
      status,
      text: async () => JSON.stringify(body),
    } as unknown as Response);
  });
}

beforeEach(() => clearToken());
afterEach(() => vi.restoreAllMocks());

describe("storeApi envelope + bearer", () => {
  it("unwraps result on ok:true", async () => {
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { version: "9.9", anon_pull: true } }));
    const r = await getInfo();
    expect(r.version).toBe("9.9");
    expect(r.anon_pull).toBe(true);
  });

  it("injects Authorization: Bearer when a token is set", async () => {
    let seen: RequestInit = {};
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { repos: [], count: 0 } }, (i) => (seen = i)));
    setToken("s3cr3t");
    await listRepos();
    const h = seen.headers as Record<string, string>;
    expect(h["Authorization"]).toBe("Bearer s3cr3t");
    expect(hasToken()).toBe(true);
  });

  it("sends no Authorization header when no token is set", async () => {
    let seen: RequestInit = {};
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { repos: [], count: 0 } }, (i) => (seen = i)));
    await listRepos();
    const h = (seen.headers as Record<string, string>) ?? {};
    expect(h["Authorization"]).toBeUndefined();
  });

  it("throws StoreApiError with status+code on ok:false", async () => {
    vi.stubGlobal("fetch", mockFetch(401, { ok: false, error: { code: "unauthorized", message: "nope" } }));
    await expect(storeApi("GET", "/v1/images")).rejects.toMatchObject({
      status: 401, code: "unauthorized", message: "nope",
    });
  });

  it("throws StoreApiError on network failure", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("boom")));
    await expect(storeApi("GET", "/v1/images")).rejects.toBeInstanceOf(StoreApiError);
  });

  it("never sends Authorization on /v1/info, even when a token is set", async () => {
    let seen: RequestInit = {};
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { version: "1.0", anon_pull: false } }, (i) => (seen = i)));
    setToken("s3cr3t");
    await getInfo();
    const h = (seen.headers as Record<string, string>) ?? {};
    expect(h["Authorization"]).toBeUndefined();
  });

  it("never puts the token in the URL for tags/manifest calls", async () => {
    let seenUrl = "";
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((url: string) => {
        seenUrl = url;
        return Promise.resolve({
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ ok: true, result: { name: "x", tags: [] } }),
        } as unknown as Response);
      }),
    );
    setToken("s3cr3t");
    await getTags("x");
    expect(seenUrl).not.toContain("s3cr3t");
  });

  it("getManifest hits the manifest path and unwraps result", async () => {
    vi.stubGlobal(
      "fetch",
      mockFetch(200, {
        ok: true,
        result: { schema_version: 1, name: "x", tag: "latest", built_at: "t", parents: [], plugins: [], requires_secrets: [], harness: { type: "cli", interactive: false }, env: {}, policy: {}, evals: [], layers: [] },
      }),
    );
    const m = await getManifest("x", "latest");
    expect(m.name).toBe("x");
    expect(m.tag).toBe("latest");
  });
});

describe("probeAuth", () => {
  it("returns true when the catalog is readable", async () => {
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { repos: [], count: 0 } }));
    expect(await probeAuth()).toBe(true);
  });
  it("returns false on 401/403", async () => {
    vi.stubGlobal("fetch", mockFetch(401, { ok: false, error: { code: "unauthorized", message: "x" } }));
    expect(await probeAuth()).toBe(false);
  });
  it("rethrows non-auth errors", async () => {
    vi.stubGlobal("fetch", mockFetch(500, { ok: false, error: { code: "boom", message: "x" } }));
    await expect(probeAuth()).rejects.toBeInstanceOf(StoreApiError);
  });
});
