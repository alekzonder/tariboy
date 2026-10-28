import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { createApi, subscribeAgentEventsOn, setActiveDaemon } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

// Capture the URL EventSource is constructed with.
class SpyES {
  static lastUrl = "";
  url: string;
  constructor(url: string) {
    this.url = url;
    SpyES.lastUrl = url;
  }
  addEventListener() {}
  removeEventListener() {}
  close() {}
}

beforeEach(() => {
  setActiveDaemon(null);
  SpyES.lastUrl = "";
  vi.stubGlobal("EventSource", SpyES as unknown as typeof EventSource);
});
afterEach(() => {
  vi.restoreAllMocks();
  setActiveDaemon(null);
});

describe("subscribeAgentEvents SSE targeting", () => {
  it("same-origin (no active daemon): relative URL, NO token query", () => {
    const stop = createApi(undefined).agents.subscribeAgentEvents("foo", ["iteration"], () => {});
    expect(SpyES.lastUrl).toBe("/api/agents/foo/events?types=iteration");
    stop();
  });

  it("cross-origin daemon: absolute baseURL + token in query", () => {
    const d: Daemon = { id: "d1", label: "prod", baseURL: "https://prod:8765", token: "t0k" };
    const stop = subscribeAgentEventsOn(d, "foo", ["iteration", "message"], () => {});
    // baseURL prefix, types preserved, token appended.
    expect(SpyES.lastUrl).toContain("https://prod:8765/api/agents/foo/events?");
    expect(SpyES.lastUrl).toContain("types=iteration%2Cmessage");
    expect(SpyES.lastUrl).toContain("token=t0k");
    stop();
  });

  it("a client subscribes on its own host with its token", () => {
    const d: Daemon = { id: "d2", label: "b", baseURL: "https://b:2/", token: "tb" };
    const stop = createApi(d).agents.subscribeAgentEvents("foo", ["audit"], () => {});
    expect(SpyES.lastUrl).toBe("https://b:2/api/agents/foo/events?types=audit&token=tb");
    stop();
  });

  it("cross-origin daemon with NO types: still carries the token", () => {
    const d: Daemon = { id: "d1", label: "p", baseURL: "https://p:1", token: "tk" };
    const stop = subscribeAgentEventsOn(d, "bar", [], () => {});
    expect(SpyES.lastUrl).toBe("https://p:1/api/agents/bar/events?token=tk");
    stop();
  });

  it("reopens the stream when the app comes back and stops after unsubscribe", () => {
    const opened: string[] = [];
    const closed = vi.fn();
    class CountingES extends SpyES {
      constructor(url: string) {
        super(url);
        opened.push(url);
      }
      close() {
        closed();
      }
    }
    vi.stubGlobal("EventSource", CountingES as unknown as typeof EventSource);
    const stop = subscribeAgentEventsOn(null, "foo", ["iteration"], () => {});
    window.dispatchEvent(new Event("online"));
    expect(closed).toHaveBeenCalledTimes(1);
    expect(opened).toEqual([
      "/api/agents/foo/events?types=iteration",
      "/api/agents/foo/events?types=iteration",
    ]);
    stop();
    expect(closed).toHaveBeenCalledTimes(2);
    window.dispatchEvent(new Event("online"));
    expect(opened).toHaveLength(2);
  });

  it("fails closed for an unresolved remote instead of opening local SSE", () => {
    setActiveDaemon({ id: "d0", label: "remote", baseURL: "", token: "" });
    const stop = createApi(undefined).agents.subscribeAgentEvents("baz", [], () => {});
    expect(SpyES.lastUrl).toBe("");
    stop();
  });
});
