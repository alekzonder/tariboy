import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { createApi, setActiveDaemon, setLocalBaseURL } from "@/lib/api";
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
  setLocalBaseURL("");
});

describe("overview agent methods", () => {
  it("setAlias posts {value} to the alias route", async () => {
    let captured: { url: string; body: unknown } | undefined;
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: { name: "a1", alias: "x" } }, (url, init) => {
        captured = { url, body: init.body ? JSON.parse(init.body as string) : undefined };
      }),
    );
    await createApi(undefined).agents.setAlias("a1", "x");
    expect(captured?.url).toContain("/api/agents/a1/alias");
    expect(captured?.body).toEqual({ value: "x" });
  });

  it("getStatusHistory reads the status/history route", async () => {
    let url = "";
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: { events: [], count: 0 } }, (u) => {
        url = u;
      }),
    );
    const r = await createApi(undefined).agents.getStatusHistory("a1");
    expect(url).toContain("/api/agents/a1/status/history");
    expect(r.count).toBe(0);
  });

  it("setAgentInteractive posts a boolean value", async () => {
    let seenInit: RequestInit = {};
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: { interactive: true } }, (_u, i) => (seenInit = i)),
    );
    await createApi(undefined).agents.setAgentInteractive("a1", true);
    expect(JSON.parse(seenInit.body as string)).toEqual({ value: true });
  });

  it("starts a created agent on the explicit host without a request body", async () => {
    let seenUrl = "";
    let seenInit: RequestInit = {};
    vi.stubGlobal(
      "fetch",
      mockFetch(200, { ok: true, result: { name: "a b", action: "start" } }, (url, init) => {
        seenUrl = url;
        seenInit = init;
      }),
    );
    const other: Daemon = { id: "d2", label: "b", baseURL: "https://b:9", token: "tb" };

    await createApi(other).agents.startAgent("a b");

    expect(seenUrl).toBe("https://b:9/api/agents/a%20b/start");
    expect(seenInit.method).toBe("POST");
    expect(seenInit.body).toBeUndefined();
  });
});

describe("subscribeAgentEvents", () => {
  it("builds the ?types= query and closes cleanly", () => {
    const urls: string[] = [];
    class Fake {
      constructor(url: string) {
        urls.push(url);
      }
      addEventListener() {}
      close() {}
    }
    vi.stubGlobal("EventSource", Fake as unknown as typeof EventSource);
    const off = createApi(undefined).agents.subscribeAgentEvents(
      "a b",
      ["iteration", "audit"],
      () => {},
    );
    expect(urls[0]).toBe("/api/agents/a%20b/events?types=iteration%2Caudit");
    off();
  });

  it("opens local SSE against the base URL", () => {
    setLocalBaseURL("http://127.0.0.1:9993");
    let opened = "";
    class CaptureES {
      constructor(u: string) {
        opened = u;
      }
      addEventListener() {}
      close() {}
    }
    vi.stubGlobal("EventSource", CaptureES);
    const off = createApi(undefined).agents.subscribeAgentEvents("bob", ["message"], () => {});
    expect(opened).toBe("http://127.0.0.1:9993/api/agents/bob/events?types=message");
    off();
  });
});
