import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { subscribeAgentEvents, setActiveDaemon } from "./api";
import type { Daemon } from "./daemons";

class SpyES {
  static urls: string[] = [];
  constructor(url: string) {
    SpyES.urls.push(url);
  }
  addEventListener() {}
  removeEventListener() {}
  close() {}
}

beforeEach(() => {
  setActiveDaemon(null);
  SpyES.urls = [];
  vi.stubGlobal("EventSource", SpyES as unknown as typeof EventSource);
});
afterEach(() => {
  vi.restoreAllMocks();
  setActiveDaemon(null);
});

describe("SSE re-targets on host switch", () => {
  it("opens same-origin, then after a switch opens the new daemon's URL with its token", () => {
    const stop1 = subscribeAgentEvents("foo", ["iteration"], () => {});
    expect(SpyES.urls[0]).toBe("/api/agents/foo/events?types=iteration");
    stop1();

    const d: Daemon = { id: "d1", label: "prod", baseURL: "https://prod:8765", token: "tp" };
    setActiveDaemon(d);
    const stop2 = subscribeAgentEvents("foo", ["iteration"], () => {});
    expect(SpyES.urls[1]).toContain("https://prod:8765/api/agents/foo/events?");
    expect(SpyES.urls[1]).toContain("token=tp");
    stop2();
  });
});
