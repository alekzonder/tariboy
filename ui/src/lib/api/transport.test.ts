import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import {
  createApi,
  setActiveDaemon,
  getActiveDaemon,
  ApiError,
  agentApiPath,
  setLocalBaseURL,
} from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

function mockFetch(
  status: number,
  body: unknown,
  capture?: (url: string, init: RequestInit) => void,
) {
  return vi.fn().mockImplementation((url: string, init: RequestInit) => {
    capture?.(url, init);
    return Promise.resolve({
      ok: status >= 200 && status < 300,
      status,
      text: async () => JSON.stringify(body),
      json: async () => body,
    } as unknown as Response);
  });
}

beforeEach(() => setActiveDaemon(null));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  setActiveDaemon(null);
});

describe("api envelope", () => {
  it("unwraps result on ok:true", async () => {
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { version: "9.9" } }));
    const r = await createApi(undefined).request<{ version: string }>("GET", "/api/daemon/status");
    expect(r.version).toBe("9.9");
  });

  it("throws ApiError with code+message on ok:false", async () => {
    vi.stubGlobal(
      "fetch",
      mockFetch(400, { ok: false, error: { code: "bad_arg", message: "nope" } }),
    );
    await expect(
      createApi(undefined).request("POST", "/api/agents", { image: "" }),
    ).rejects.toMatchObject({
      code: "bad_arg",
      message: "nope",
    });
  });

  it("throws ApiError on network failure", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("boom")));
    await expect(createApi(undefined).request("GET", "/api/agents")).rejects.toBeInstanceOf(
      ApiError,
    );
  });

  it("treats an empty non-2xx body as an error envelope", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 500, text: async () => "" } as unknown as Response),
    );
    await expect(createApi(undefined).request("GET", "/api/agents")).rejects.toMatchObject({
      status: 500,
    });
  });
});

describe("agentApiPath", () => {
  it("url-encodes the name and strips leading slashes on rest", () => {
    expect(agentApiPath("a b", "status")).toBe("/api/agents/a%20b/status");
    expect(agentApiPath("x", "/iterations")).toBe("/api/agents/x/iterations");
    expect(agentApiPath("x", "")).toBe("/api/agents/x");
  });

  it("strips a leading api/ on the rest segment", () => {
    expect(agentApiPath("x", "api/secrets")).toBe("/api/agents/x/secrets");
  });
});

describe("api backward-compat (empty registry = same-origin, no auth)", () => {
  it("with NO active daemon: relative path, NO Authorization header", async () => {
    let seenUrl = "";
    let seenInit: RequestInit = {};
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: { version: "9" } }, (u, i) => {
        seenUrl = u;
        seenInit = i;
      }),
    );
    await createApi(undefined).get("/api/daemon/status");
    expect(seenUrl).toBe("/api/daemon/status"); // relative — exactly M11
    const h = (seenInit.headers as Record<string, string>) ?? {};
    expect(h["Authorization"]).toBeUndefined();
    expect(getActiveDaemon()).toBeNull();
  });
});

describe("api with an active daemon (baseURL + bearer)", () => {
  const d: Daemon = { id: "d1", label: "prod", baseURL: "https://prod:8765", token: "t0k" };

  it("prepends baseURL and injects Authorization: Bearer", async () => {
    let seenUrl = "";
    let seenInit: RequestInit = {};
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: { version: "9" } }, (u, i) => {
        seenUrl = u;
        seenInit = i;
      }),
    );
    setActiveDaemon(d);
    await createApi(undefined).get("/api/daemon/status");
    expect(seenUrl).toBe("https://prod:8765/api/daemon/status");
    const h = seenInit.headers as Record<string, string>;
    expect(h["Authorization"]).toBe("Bearer t0k");
  });

  it("a client for an explicit daemon does not change the active one", async () => {
    let seenUrl = "";
    let seenInit: RequestInit = {};
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: { count: 0 } }, (u, i) => {
        seenUrl = u;
        seenInit = i;
      }),
    );
    const other: Daemon = { id: "d2", label: "b", baseURL: "https://b:9", token: "tb" };
    await createApi(other).request("GET", "/api/agents");
    expect(seenUrl).toBe("https://b:9/api/agents");
    expect((seenInit.headers as Record<string, string>)["Authorization"]).toBe("Bearer tb");
    expect(getActiveDaemon()).toBeNull(); // unchanged
  });

  it("fails closed instead of targeting local when a remote daemon is unresolved", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    await expect(
      createApi({ id: "d3", label: "x", baseURL: "", token: "" }).request("GET", "/api/agents"),
    ).rejects.toMatchObject({ status: 0, code: "host_not_ready" });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("still throws ApiError with status+code on ok:false", async () => {
    vi.stubGlobal(
      "fetch",
      mockFetch(401, { ok: false, error: { code: "unauthorized", message: "nope" } }),
    );
    setActiveDaemon(d);
    await expect(createApi(undefined).request("GET", "/api/agents")).rejects.toMatchObject({
      status: 401,
      code: "unauthorized",
    });
  });
});

describe("local daemon base URL", () => {
  afterEach(() => setLocalBaseURL(""));

  it("prefixes local calls once set", async () => {
    setLocalBaseURL("http://127.0.0.1:9993");
    let url = "";
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: {} }, (u) => {
        url = u;
      }),
    );
    await createApi(undefined).get("/api/agents");
    expect(url).toBe("http://127.0.0.1:9993/api/agents");
  });

  it("trims trailing slashes so paths never double up", async () => {
    setLocalBaseURL("http://127.0.0.1:9993///");
    let url = "";
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: {} }, (u) => {
        url = u;
      }),
    );
    await createApi(undefined).get("/api/agents");
    expect(url).toBe("http://127.0.0.1:9993/api/agents");
  });

  it("does not touch calls addressed at a registered remote daemon", async () => {
    setLocalBaseURL("http://127.0.0.1:9993");
    const remote: Daemon = { id: "d1", label: "box", baseURL: "https://box.example", token: "t" };
    let url = "";
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: {} }, (u) => {
        url = u;
      }),
    );
    await createApi(remote).request("GET", "/api/agents");
    expect(url).toBe("https://box.example/api/agents");
  });
});
