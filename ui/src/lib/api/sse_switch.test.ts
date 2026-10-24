import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createApi, setActiveDaemon } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

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
  it("opens same-origin, then a client created after a switch opens the new daemon's URL with its token", () => {
    const before = createApi(undefined);
    const stop1 = before.agents.subscribeAgentEvents("foo", ["iteration"], () => {});
    expect(SpyES.urls[0]).toBe("/api/agents/foo/events?types=iteration");
    stop1();

    const d: Daemon = { id: "d1", label: "prod", baseURL: "https://prod:8765", token: "tp" };
    setActiveDaemon(d);
    const stop2 = createApi(undefined).agents.subscribeAgentEvents("foo", ["iteration"], () => {});
    expect(SpyES.urls[1]).toContain("https://prod:8765/api/agents/foo/events?");
    expect(SpyES.urls[1]).toContain("token=tp");
    stop2();

    // A client that already exists stays on the host it was created for.
    const stop3 = before.agents.subscribeAgentEvents("foo", ["iteration"], () => {});
    expect(SpyES.urls[2]).toBe("/api/agents/foo/events?types=iteration");
    stop3();
  });
});
