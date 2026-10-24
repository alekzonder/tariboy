import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { createApi, setActiveDaemon } from "@/lib/api";

function mockFetch(status: number, body: unknown, capture?: (url: string, init: RequestInit) => void) {
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

describe("serverUploadFile", () => {
  it("streams the File to the raw upload endpoint", async () => {
    let seenInit: RequestInit = {};
    let seenURL = "";
    vi.stubGlobal("fetch", mockFetch(200, {
      ok: true,
      result: { path: "files/one/x.txt", abs: "/server/files/one/x.txt", bytes: 2 },
    }, (u, i) => { seenURL = u; seenInit = i; }));
    const arrayBuffer = vi.fn();
    const file = { name: "x #.txt", size: 2, arrayBuffer } as unknown as File;
    const res = await createApi(undefined).files.serverUploadFile(file);
    expect(seenURL).toBe("/api/files/raw?name=x%20%23.txt");
    expect(seenInit.method).toBe("PUT");
    expect(seenInit.body).toBe(file);
    expect(arrayBuffer).not.toHaveBeenCalled();
    expect(res.abs).toBe("/server/files/one/x.txt");
  });

  it("accepts files above the former 16 MiB limit", async () => {
    vi.stubGlobal("fetch", mockFetch(200, {
      ok: true,
      result: { path: "files/one/x.bin", abs: "/server/files/one/x.bin", bytes: 1 },
    }));
    const arrayBuffer = vi.fn().mockResolvedValue(new Uint8Array([0]).buffer);
    const file = {
      name: "x.bin",
      size: 17 * 1024 * 1024,
      arrayBuffer,
    } as unknown as File;

    await createApi(undefined).files.serverUploadFile(file);
    expect(arrayBuffer).not.toHaveBeenCalled();
    expect(fetch).toHaveBeenCalledOnce();
  });

  it("rejects files above 1 GiB before reading them", async () => {
    const arrayBuffer = vi.fn();
    const file = {
      name: "x.bin",
      size: 1024 * 1024 * 1024 + 1,
      arrayBuffer,
    } as unknown as File;

    await expect(createApi(undefined).files.serverUploadFile(file)).rejects.toThrow("File exceeds 1 GiB");
    expect(arrayBuffer).not.toHaveBeenCalled();
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
    const r = await createApi(undefined).files.fsList("/home/u space");
    expect(seenUrl).toBe("/api/fs/list?path=%2Fhome%2Fu%20space");
    expect(r.entries).toHaveLength(2);
    expect(r.entries[0]).toEqual({ name: "projects", dir: true });
  });

  it("omits the query param entirely for an empty path (root)", async () => {
    let seenUrl = "";
    vi.stubGlobal("fetch", mockFetch(200, { ok: true, result: listing }, (u) => (seenUrl = u)));
    await createApi(undefined).files.fsList("");
    expect(seenUrl).toBe("/api/fs/list");
  });

  it("surfaces the typed error envelope (bad_path → 403)", async () => {
    vi.stubGlobal("fetch", mockFetch(403, { ok: false, error: { code: "bad_path", message: "outside root" } }));
    await expect(createApi(undefined).files.fsList("/etc")).rejects.toMatchObject({ status: 403, code: "bad_path" });
  });
});
