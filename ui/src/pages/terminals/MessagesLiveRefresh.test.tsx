import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocalBaseURL } from "@/lib/api";
import { MessagesLiveRefresh } from "./MessagesLiveRefresh";

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

const statesSocket = () =>
  FakeWebSocket.instances.filter((s) => s.url.includes("/api/agent-states/ws")).at(-1)!;

beforeEach(() => {
  FakeWebSocket.instances = [];
  setLocalBaseURL("");
  vi.stubGlobal("WebSocket", FakeWebSocket);
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("MessagesLiveRefresh", () => {
  it("is live only while the agent states socket is open, so the caller falls back to the short poll", () => {
    const onLive = vi.fn();
    render(<MessagesLiveRefresh hostIds={[""]} onChange={vi.fn()} onLive={onLive} />);
    expect(onLive).toHaveBeenLastCalledWith(false);

    act(() => {
      statesSocket().onopen?.(new Event("open"));
    });
    expect(onLive).toHaveBeenLastCalledWith(true);

    // An old host answers 404, which closes the socket.
    act(() => {
      statesSocket().onclose?.(new CloseEvent("close"));
    });
    expect(onLive).toHaveBeenLastCalledWith(false);
  });

  it("refreshes once for a burst of agent state hints", () => {
    const onChange = vi.fn();
    render(<MessagesLiveRefresh hostIds={[""]} onChange={onChange} />);
    act(() => {
      for (let i = 0; i < 5; i++) {
        statesSocket().onmessage?.(
          new MessageEvent("message", {
            data: JSON.stringify({ agent: "worker", type: "state", ts: "t" }),
          }),
        );
      }
    });
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(onChange).toHaveBeenCalledTimes(1);
  });
});
