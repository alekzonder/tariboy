import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type PropsWithChildren } from "react";
import { createApi, setLocalBaseURL, type ApiClient } from "@/lib/api";
import { ApiProvider } from "@/components/ApiProvider";
import type { Daemon } from "@/lib/daemons";
import { mailWsUrl, useAgentStatesSocket, useMailSocket } from "./useMailSocket";

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  readonly url: string;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  close = vi.fn();
  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }
}

beforeEach(() => {
  FakeWebSocket.instances = [];
  setLocalBaseURL("");
  vi.stubGlobal("WebSocket", FakeWebSocket);
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  setLocalBaseURL("");
});

describe("mail websocket", () => {
  it("builds local and remote URLs with the existing token convention", () => {
    expect(mailWsUrl(null)).toMatch(/^ws:\/\/.*\/api\/mail\/ws$/);
    const remote: Daemon = {
      id: "prod",
      label: "Production",
      baseURL: "https://chat.example.test/",
      token: "secret token",
    };
    expect(mailWsUrl(remote)).toBe("wss://chat.example.test/api/mail/ws?token=secret+token");
  });

  it("delivers hints and refetches on every (re)connect instead of replaying", () => {
    vi.useFakeTimers();
    const onHint = vi.fn();
    const onOpen = vi.fn();
    renderHook(() => useMailSocket({ target: null, onHint, onOpen }));
    const first = FakeWebSocket.instances[0];

    act(() => {
      first.onopen?.(new Event("open"));
    });
    expect(onOpen).toHaveBeenCalledTimes(1);

    act(() => {
      first.onmessage?.(
        new MessageEvent("message", {
          data: JSON.stringify({
            id: "m-1",
            from: "agent:worker",
            to: "user:customer",
            kind: "task.question",
            ts: "t",
          }),
        }),
      );
      first.onmessage?.(new MessageEvent("message", { data: "not json" }));
      first.onmessage?.(new MessageEvent("message", { data: JSON.stringify({ from: "agent:x" }) }));
    });
    expect(onHint).toHaveBeenCalledTimes(1);
    expect(onHint.mock.calls[0][0]).toMatchObject({
      id: "m-1",
      to: "user:customer",
      kind: "task.question",
    });

    act(() => {
      first.onclose?.(new CloseEvent("close"));
    });
    act(() => {
      vi.advanceTimersByTime(300);
    });
    const second = FakeWebSocket.instances[1];
    expect(second).toBeDefined();
    act(() => {
      second.onopen?.(new Event("open"));
    });
    expect(onOpen).toHaveBeenCalledTimes(2);
  });

  it("redials at once when the app comes back, and the new socket refetches", () => {
    vi.useFakeTimers();
    const onOpen = vi.fn();
    renderHook(() => useMailSocket({ target: null, onHint: vi.fn(), onOpen }));
    const first = FakeWebSocket.instances[0];
    act(() => {
      first.onopen?.(new Event("open"));
    });

    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    expect(first.close).toHaveBeenCalledOnce();
    const second = FakeWebSocket.instances[1];
    expect(second).toBeDefined();
    act(() => {
      second.onopen?.(new Event("open"));
    });
    expect(onOpen).toHaveBeenCalledTimes(2);

    // The superseded socket's close must not schedule another dial.
    act(() => {
      first.onclose?.(new CloseEvent("close"));
      vi.runAllTimers();
    });
    expect(FakeWebSocket.instances).toHaveLength(2);
  });
});

describe("agent states websocket", () => {
  it("connects to the agent states socket and reports open", () => {
    const { result } = renderHook(() => useAgentStatesSocket({ target: null, onHint: vi.fn() }));
    const socket = FakeWebSocket.instances[0];
    expect(socket.url).toMatch(/^ws:\/\/.*\/api\/agent-states\/ws$/);
    act(() => {
      socket.onopen?.(new Event("open"));
    });
    expect(result.current).toBe("open");
  });

  it("delivers frames that name an agent and drops the rest", () => {
    const onHint = vi.fn();
    renderHook(() => useAgentStatesSocket({ target: null, onHint }));
    const socket = FakeWebSocket.instances[0];
    act(() => {
      socket.onmessage?.(
        new MessageEvent("message", {
          data: JSON.stringify({ agent: "worker", type: "iteration", ts: "t" }),
        }),
      );
      socket.onmessage?.(
        new MessageEvent("message", { data: JSON.stringify({ type: "iteration" }) }),
      );
    });
    expect(onHint).toHaveBeenCalledTimes(1);
    expect(onHint.mock.calls[0][0].agent).toBe("worker");
  });
});

describe("socket host", () => {
  const remote: Daemon = { id: "r", label: "Remote", baseURL: "https://r.example", token: "one" };

  it("watches the provider's host and survives ordinary re-renders", () => {
    let client: ApiClient = createApi(remote);
    const { rerender } = renderHook(() => useMailSocket({ onHint: vi.fn() }), {
      wrapper: ({ children }: PropsWithChildren) =>
        // oxlint-disable-next-line react/no-children-prop -- ApiProvider types children as a required prop, so createElement passes it there
        createElement(ApiProvider, { client, children }),
    });
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].url).toBe("wss://r.example/api/mail/ws?token=one");
    client = createApi({ ...remote });
    rerender();
    expect(FakeWebSocket.instances).toHaveLength(1);
    client = createApi({ ...remote, token: "two" });
    rerender();
    expect(FakeWebSocket.instances).toHaveLength(2);
    expect(FakeWebSocket.instances[0].close).toHaveBeenCalled();
    expect(FakeWebSocket.instances[1].url).toContain("token=two");
  });
});
