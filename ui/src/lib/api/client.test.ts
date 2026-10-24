import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, createApi, setActiveDaemon, setLocalBaseURL } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

const hostA: Daemon = { id: "a", label: "A", baseURL: "https://a.example:9443", token: "tok-a" };
const hostB: Daemon = { id: "b", label: "B", baseURL: "https://b.example:9443/", token: "tok-b" };

type Call = { url: string; init: RequestInit };

function stubFetch(status = 200, body: unknown = { ok: true, result: {} }) {
  const calls: Call[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string, init: RequestInit) => {
    calls.push({ url, init });
    return Promise.resolve({
      ok: status >= 200 && status < 300,
      status,
      text: async () => (typeof body === "string" ? body : JSON.stringify(body)),
      json: async () => body,
    } as unknown as Response);
  }));
  return calls;
}

const auth = (call: Call) => (call.init.headers as Record<string, string> | undefined)?.Authorization;

beforeEach(() => setActiveDaemon(null));
afterEach(() => {
  vi.unstubAllGlobals();
  setActiveDaemon(null);
  setLocalBaseURL("");
});

describe("createApi", () => {
  it("binds each client to its own host even after the global selection changes", async () => {
    const calls = stubFetch();
    const a = createApi(hostA);
    const b = createApi(hostB);
    setActiveDaemon(hostB);
    await a.tasks.getTask("T-1");
    await b.tasks.getTask("T-2");
    expect(calls[0].url).toBe("https://a.example:9443/api/tasks/T-1");
    expect(auth(calls[0])).toBe("Bearer tok-a");
    expect(calls[1].url).toBe("https://b.example:9443/api/tasks/T-2");
    expect(auth(calls[1])).toBe("Bearer tok-b");
  });

  it("resolves undefined to the active daemon once, at creation", async () => {
    const calls = stubFetch();
    setActiveDaemon(hostA);
    const api = createApi(undefined);
    setActiveDaemon(hostB);
    await api.agents.listAgents();
    expect(api.target).toBe(hostA);
    expect(calls[0].url).toBe("https://a.example:9443/api/agents");
  });

  it("treats explicit null as the local daemon without a remote token", async () => {
    const calls = stubFetch();
    setActiveDaemon(hostA);
    setLocalBaseURL("http://127.0.0.1:7000/");
    const api = createApi(null);
    await api.daemon.getDaemonStatus();
    expect(api.target).toBeNull();
    expect(calls[0].url).toBe("http://127.0.0.1:7000/api/daemon/status");
    expect(auth(calls[0])).toBeUndefined();
  });

  it("refuses a remote host without an endpoint and never falls back to local", async () => {
    const calls = stubFetch();
    const api = createApi({ ...hostA, baseURL: "" });
    await expect(api.images.listImages()).rejects.toMatchObject({ code: "host_not_ready" });
    await expect(api.raw("GET", "/api/x")).rejects.toMatchObject({ code: "host_not_ready" });
    expect(calls).toHaveLength(0);
    expect(api.ready).toBe(false);
    expect(createApi(hostA).ready).toBe(true);
    expect(createApi(null).ready).toBe(true);
  });

  it("keeps method, body, cache, signal and error fields of the transport", async () => {
    const calls = stubFetch(409, { ok: false, error: { code: "conflict", message: "stale", details: { revision: 3 } } });
    const controller = new AbortController();
    const api = createApi(hostA);
    const err = await api.request("PATCH", "/api/tasks/T-1", { revision: 2 }, controller.signal).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: "conflict", message: "stale", details: { revision: 3 } });
    expect(calls[0].init).toMatchObject({ method: "PATCH", cache: "no-store", body: JSON.stringify({ revision: 2 }), signal: controller.signal });
    expect((calls[0].init.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
  });

  it("reports empty and malformed responses", async () => {
    stubFetch(500, "");
    await expect(createApi(null).get("/api/x")).rejects.toMatchObject({ status: 500, code: "http_500" });
    stubFetch(200, "<html>");
    await expect(createApi(null).get("/api/x")).rejects.toMatchObject({ code: "bad_response" });
  });

  it("uploads the original File through the raw transport", async () => {
    const calls = stubFetch(200, { ok: true, result: { path: "a.txt", abs: "/h/a.txt", bytes: 3 } });
    const file = new File(["abc"], "a.txt");
    const result = await createApi(hostA).files.serverUploadFile(file);
    expect(result.bytes).toBe(3);
    expect(calls[0].url).toBe("https://a.example:9443/api/files/raw?name=a.txt");
    expect(calls[0].init.body).toBe(file);
    expect(auth(calls[0])).toBe("Bearer tok-a");
  });

  it("rejects a file above 1 GiB before any request", async () => {
    const calls = stubFetch();
    const big = { name: "big", size: 1024 * 1024 * 1024 + 1 } as File;
    await expect(createApi(null).files.serverUploadFile(big)).rejects.toThrow("File exceeds 1 GiB");
    expect(calls).toHaveLength(0);
  });
});
