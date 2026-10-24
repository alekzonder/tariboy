import { describe, it, expect, vi, afterEach } from "vitest";
import { createApi, getActiveDaemon, getLocalBaseURL, resolveTarget, setActiveDaemon, setLocalBaseURL } from "@/lib/api";

afterEach(() => {
  vi.unstubAllGlobals();
  setActiveDaemon(null);
  setLocalBaseURL("");
});

describe("active daemon", () => {
  it("resolveTarget: undefined → active daemon, null → same-origin, object → itself", () => {
    const d = { id: "d1", label: "x", baseURL: "https://h", token: "t" };
    setActiveDaemon(d);
    expect(getActiveDaemon()).toBe(d);
    expect(resolveTarget(undefined)).toBe(d);
    expect(resolveTarget(null)).toBeNull();
    const other = { id: "d2", label: "y", baseURL: "https://h2", token: "t2" };
    expect(resolveTarget(other)).toBe(other);
    setActiveDaemon(null);
    expect(getActiveDaemon()).toBeNull();
  });
});

describe("local daemon base URL", () => {
  it("defaults to empty, keeping same-origin relative paths", async () => {
    let url = "";
    vi.stubGlobal("fetch", vi.fn().mockImplementation((u: string) => {
      url = u;
      return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: {} }) } as Response);
    }));
    await createApi(undefined).get("/api/agents");
    expect(url).toBe("/api/agents");
    expect(getLocalBaseURL()).toBe("");
  });

  it("trims trailing slashes when set", () => {
    setLocalBaseURL("http://127.0.0.1:9993///");
    expect(getLocalBaseURL()).toBe("http://127.0.0.1:9993");
  });
});
