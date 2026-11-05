import { render } from "@testing-library/react";
import { composeStories, setProjectAnnotations } from "@storybook/react-vite";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import * as preview from "../../.storybook/preview";

// Storybook never reaches a daemon (spec 9.1). Mount every story and record the
// URL of each fetch and WebSocket. A relative URL (it would hit /api on the page
// origin) and the live listener 127.0.0.1:9990 fail the test. A request to
// storybook.invalid is allowed: the name does not resolve.
const modules = import.meta.glob<Record<string, unknown>>("../**/*.stories.tsx", { eager: true });

const requests: string[] = [];
const record = (url: unknown) => {
  requests.push(url instanceof Request ? url.url : String(url));
};

beforeAll(() => {
  setProjectAnnotations(preview.default);
  // Same define as vite.storybook.config.ts.
  vi.stubEnv("VITE_TARIBOY_NO_LOCAL_HOST", "1");
});
afterEach(() => {
  vi.unstubAllGlobals();
});

const isAllowed = (url: string) => /^(https?|wss?):\/\/storybook\.invalid(\/|$)/.test(url);

it("mounts every story without a relative or loopback request", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: unknown) => {
      record(input);
      return Promise.reject(new TypeError("blocked by storyNetwork.test"));
    }),
  );
  // An inert socket: it records the URL and never opens or fires an event.
  class BlockedSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;
    readyState = 3;
    constructor(url: string | URL) {
      record(url);
    }
    close() {}
    send() {}
    addEventListener() {}
    removeEventListener() {}
  }
  vi.stubGlobal("WebSocket", BlockedSocket);

  const names: string[] = [];
  for (const [file, mod] of Object.entries(modules)) {
    const stories = composeStories(mod as never) as Record<string, React.ComponentType>;
    for (const [name, Story] of Object.entries(stories)) {
      const { unmount } = render(<Story />);
      // Let effects and query fetches start, then fail.
      await new Promise((resolve) => setTimeout(resolve, 20));
      unmount();
      names.push(`${file}:${name}`);
    }
  }
  expect(names.length).toBeGreaterThan(40);
  expect(requests.filter((url) => !isAllowed(url))).toEqual([]);
});
