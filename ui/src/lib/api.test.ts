import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import {
  api, apiGet, apiOn, setActiveDaemon, getActiveDaemon, ApiError, agentApiPath, fsList,
  setLocalBaseURL, getLocalBaseURL, listImagesOn, imageManifestGetOn, imagePromptGetOn,
  imageFilesListOn, imageFileReadOn, startAgent,
} from "./api";
import { subscribeAgentEvents, agentUploadFile, setAgentInteractive } from "./api";
import { setAlias, getStatusHistory, loopEnable } from "./api";
import type { Daemon } from "./daemons";

function mockFetch(status: number, body: unknown, capture?: (url: string, init: RequestInit) => void) {
  return vi.fn().mockImplementation((url: string, init: RequestInit) => {
    capture?.(url, init);
    return Promise.resolve({
      ok: status >= 200 && status < 300,
      status,
      text: async () => JSON.stringify(body),
    } as unknown as Response);
  });
}

beforeEach(() => setActiveDaemon(null));
afterEach(() => {
  vi.restoreAllMocks();
  setActiveDaemon(null);
});

describe("api envelope", () => {
  it("unwraps result on ok:true", async () => {
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { version: "9.9" } }));
    const r = await api<{ version: string }>("GET", "/api/daemon/status");
    expect(r.version).toBe("9.9");
  });

  it("throws ApiError with code+message on ok:false", async () => {
    vi.stubGlobal("fetch", mockFetch(400, { ok: false, error: { code: "bad_arg", message: "nope" } }));
    await expect(api("POST", "/api/agents", { image: "" })).rejects.toMatchObject({
      code: "bad_arg",
      message: "nope",
    });
  });

  it("throws ApiError on network failure", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("boom")));
    await expect(api("GET", "/api/agents")).rejects.toBeInstanceOf(ApiError);
  });
});

describe("overview api helpers", () => {
  it("setAlias posts {value} to the alias route", async () => {
    let captured: { url: string; body: unknown } | undefined;
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { name: "a1", alias: "x" } }, (url, init) => {
      captured = { url, body: init.body ? JSON.parse(init.body as string) : undefined };
    }));
    await setAlias("a1", "x");
    expect(captured?.url).toContain("/api/agents/a1/alias");
    expect(captured?.body).toEqual({ value: "x" });
  });

  it("getStatusHistory reads the status/history route", async () => {
    let url = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { events: [], count: 0 } }, (u) => { url = u; }));
    const r = await getStatusHistory("a1");
    expect(url).toContain("/api/agents/a1/status/history");
    expect(r.count).toBe(0);
  });

  it("loopEnable posts to the loop/enable route", async () => {
    let url = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { loop_enabled: true } }, (u) => { url = u; }));
    await loopEnable("a1");
    expect(url).toContain("/api/agents/a1/loop/enable");
  });
});

describe("agentApiPath", () => {
  it("url-encodes the name and strips leading slashes on rest", () => {
    expect(agentApiPath("a b", "status")).toBe("/api/agents/a%20b/status");
    expect(agentApiPath("x", "/iterations")).toBe("/api/agents/x/iterations");
    expect(agentApiPath("x", "")).toBe("/api/agents/x");
  });
});

describe("api extras (M11b carry-forward)", () => {
  it("agentApiPath strips a leading api/ on the rest segment", () => {
    expect(agentApiPath("x", "api/secrets")).toBe("/api/agents/x/secrets");
  });

  it("subscribeAgentEvents builds the ?types= query and closes cleanly", () => {
    const urls: string[] = [];
    class Fake {
      constructor(url: string) { urls.push(url); }
      addEventListener() {}
      close() {}
    }
    vi.stubGlobal("EventSource", Fake as unknown as typeof EventSource);
    const off = subscribeAgentEvents("a b", ["iteration", "audit"], () => {});
    expect(urls[0]).toBe("/api/agents/a%20b/events?types=iteration%2Caudit");
    off();
  });

  it("treats an empty non-2xx body as an error envelope", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 500, text: async () => "" } as unknown as Response));
    await expect(api("GET", "/api/agents")).rejects.toMatchObject({ status: 500 });
  });
});

describe("interactive terminal + upload helpers", () => {
  it("agentUploadFile targets .tariboy/files and returns abs", async () => {
    let seenInit: RequestInit = {};
    vi.stubGlobal("fetch", mockFetch(200, {
      ok: true,
      result: { path: ".tariboy/files/x.txt", abs: "/cwd/.tariboy/files/x.txt", bytes: 2 },
    }, (_u, i) => (seenInit = i)));
    const file = new File(["hi"], "x.txt");
    const res = await agentUploadFile("a1", file);
    const body = JSON.parse(seenInit.body as string);
    expect(seenInit.method).toBe("PUT");
    expect(body.path).toBe(".tariboy/files/x.txt");
    expect(body.content).toBe(btoa("hi"));
    expect(res.abs).toContain(".tariboy/files/x.txt");
  });

  it("setAgentInteractive posts a boolean value", async () => {
    let seenInit: RequestInit = {};
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { interactive: true } }, (_u, i) => (seenInit = i)));
    await setAgentInteractive("a1", true);
    expect(JSON.parse(seenInit.body as string)).toEqual({ value: true });
  });
});

describe("api backward-compat (empty registry = same-origin, no auth)", () => {
  it("with NO active daemon: relative path, NO Authorization header", async () => {
    let seenUrl = "";
    let seenInit: RequestInit = {};
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { version: "9" } }, (u, i) => {
      seenUrl = u;
      seenInit = i;
    }));
    await apiGet("/api/daemon/status");
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
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { version: "9" } }, (u, i) => {
      seenUrl = u;
      seenInit = i;
    }));
    setActiveDaemon(d);
    await apiGet("/api/daemon/status");
    expect(seenUrl).toBe("https://prod:8765/api/daemon/status");
    const h = seenInit.headers as Record<string, string>;
    expect(h["Authorization"]).toBe("Bearer t0k");
  });

  it("apiOn targets an explicit daemon without changing the active one", async () => {
    let seenUrl = "";
    let seenInit: RequestInit = {};
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: { count: 0 } }, (u, i) => {
      seenUrl = u;
      seenInit = i;
    }));
    const other: Daemon = { id: "d2", label: "b", baseURL: "https://b:9", token: "tb" };
    await apiOn(other, "GET", "/api/agents");
    expect(seenUrl).toBe("https://b:9/api/agents");
    expect((seenInit.headers as Record<string, string>)["Authorization"]).toBe("Bearer tb");
    expect(getActiveDaemon()).toBeNull(); // unchanged
  });

  it("starts a created agent on the explicit host without a request body", async () => {
    let seenUrl = "";
    let seenInit: RequestInit = {};
    vi.stubGlobal("fetch", mockFetch(
      200,
      { ok: true, result: { name: "a b", action: "start" } },
      (url, init) => {
        seenUrl = url;
        seenInit = init;
      },
    ));
    const other: Daemon = { id: "d2", label: "b", baseURL: "https://b:9", token: "tb" };

    await startAgent("a b", other);

    expect(seenUrl).toBe("https://b:9/api/agents/a%20b/start");
    expect(seenInit.method).toBe("POST");
    expect(seenInit.body).toBeUndefined();
  });

  it("fails closed instead of targeting local when a remote daemon is unresolved", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    await expect(
      apiOn({ id: "d3", label: "x", baseURL: "", token: "" }, "GET", "/api/agents"),
    ).rejects.toMatchObject({ status: 0, code: "host_not_ready" });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("still throws ApiError with status+code on ok:false", async () => {
    vi.stubGlobal("fetch", mockFetch(401, { ok: false, error: { code: "unauthorized", message: "nope" } }));
    setActiveDaemon(d);
    await expect(api("GET", "/api/agents")).rejects.toMatchObject({ status: 401, code: "unauthorized" });
    void ApiError;
  });
});

describe("fsList (cwd path autocomplete)", () => {
  const listing = {
    path: "/home/u",
    parent: "/home",
    entries: [{ name: "projects", dir: true }, { name: ".config", dir: true }],
  };

  it("GETs /api/fs/list with the path url-encoded and returns the listing", async () => {
    let seenUrl = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: listing }, (u) => (seenUrl = u)));
    const r = await fsList("/home/u space");
    expect(seenUrl).toBe("/api/fs/list?path=%2Fhome%2Fu%20space");
    expect(r.entries).toHaveLength(2);
    expect(r.entries[0]).toEqual({ name: "projects", dir: true });
  });

  it("omits the query param entirely for an empty path (root)", async () => {
    let seenUrl = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: listing }, (u) => (seenUrl = u)));
    await fsList("");
    expect(seenUrl).toBe("/api/fs/list");
  });

  it("surfaces the typed error envelope (bad_path → 403)", async () => {
    vi.stubGlobal("fetch", mockFetch(403, { ok: false, error: { code: "bad_path", message: "outside root" } }));
    await expect(fsList("/etc")).rejects.toMatchObject({ status: 403, code: "bad_path" });
  });
});

describe("local daemon base URL", () => {
  afterEach(() => setLocalBaseURL(""));

  it("defaults to empty, keeping same-origin relative paths", async () => {
    let url = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: {} }, (u) => { url = u; }));
    await apiGet("/api/agents");
    expect(url).toBe("/api/agents");
    expect(getLocalBaseURL()).toBe("");
  });

  it("prefixes local calls once set", async () => {
    setLocalBaseURL("http://127.0.0.1:9993");
    let url = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: {} }, (u) => { url = u; }));
    await apiGet("/api/agents");
    expect(url).toBe("http://127.0.0.1:9993/api/agents");
  });

  it("trims trailing slashes so paths never double up", async () => {
    setLocalBaseURL("http://127.0.0.1:9993///");
    let url = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: {} }, (u) => { url = u; }));
    await apiGet("/api/agents");
    expect(url).toBe("http://127.0.0.1:9993/api/agents");
  });

  it("does not touch calls addressed at a registered remote daemon", async () => {
    setLocalBaseURL("http://127.0.0.1:9993");
    const remote: Daemon = { id: "d1", label: "box", baseURL: "https://box.example", token: "t" };
    let url = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: {} }, (u) => { url = u; }));
    await apiOn(remote, "GET", "/api/agents");
    expect(url).toBe("https://box.example/api/agents");
  });

  it("opens local SSE against the base URL", () => {
    setLocalBaseURL("http://127.0.0.1:9993");
    let opened = "";
    class CaptureES {
      constructor(u: string) { opened = u; }
      addEventListener() {}
      close() {}
    }
    vi.stubGlobal("EventSource", CaptureES);
    const off = subscribeAgentEvents("bob", ["message"], () => {});
    expect(opened).toBe("http://127.0.0.1:9993/api/agents/bob/events?types=message");
    off();
  });
});

describe("per-target helpers", () => {
  it("resolveTarget: undefined → active daemon, null → same-origin, object → itself", async () => {
    const { resolveTarget, setActiveDaemon } = await import("./api");
    const d = { id: "d1", label: "x", baseURL: "https://h", token: "t" };
    setActiveDaemon(d);
    expect(resolveTarget(undefined)).toBe(d);
    expect(resolveTarget(null)).toBeNull();
    const other = { id: "d2", label: "y", baseURL: "https://h2", token: "t2" };
    expect(resolveTarget(other)).toBe(other);
    setActiveDaemon(null);
  });

  it("addresses every built-image read through the explicit target", async () => {
    const calls: Array<{ url: string; method: string }> = [];
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: {} }, (url, init) => {
      calls.push({ url, method: init.method ?? "GET" });
    }));
    const d = { id: "d1", label: "x", baseURL: "https://images.example/", token: "t" };

    await listImagesOn(d);
    await imageManifestGetOn(d, "reviewer:latest");
    await imagePromptGetOn(d, "reviewer:latest");
    await imageFilesListOn(d, "reviewer:latest");
    await imageFileReadOn(d, "reviewer:latest", "skills/a b/#review.md");

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
