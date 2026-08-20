import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { subscribeAgentEvents, subscribeAgentEventsOn, setActiveDaemon } from "./api";
import type { Daemon } from "./daemons";

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
    const stop = subscribeAgentEvents("foo", ["iteration"], () => {});
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

  it("cross-origin daemon with NO types: still carries the token", () => {
    const d: Daemon = { id: "d1", label: "p", baseURL: "https://p:1", token: "tk" };
    const stop = subscribeAgentEventsOn(d, "bar", [], () => {});
    expect(SpyES.lastUrl).toBe("https://p:1/api/agents/bar/events?token=tk");
    stop();
  });

  it("fails closed for an unresolved remote instead of opening local SSE", () => {
    setActiveDaemon({ id: "d0", label: "remote", baseURL: "", token: "" });
    const stop = subscribeAgentEvents("baz", [], () => {});
    expect(SpyES.lastUrl).toBe("");
    stop();
  });
});
