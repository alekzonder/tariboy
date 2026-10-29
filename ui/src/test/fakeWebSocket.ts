import { act } from "@testing-library/react";
import { vi } from "vitest";

/** A WebSocket stand-in that never dials: a test opens it and sends frames by hand. */
export class FakeWebSocket {
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

  /** Replace the global WebSocket for one test; undo with vi.unstubAllGlobals. */
  static install() {
    FakeWebSocket.instances = [];
    vi.stubGlobal("WebSocket", FakeWebSocket);
  }

  /** The newest socket dialed to `path`. */
  static last(path: string): FakeWebSocket | undefined {
    return FakeWebSocket.instances.filter((socket) => socket.url.includes(path)).at(-1);
  }

  open() {
    act(() => this.onopen?.(new Event("open")));
  }

  send(frame: unknown) {
    act(() => this.onmessage?.(new MessageEvent("message", { data: JSON.stringify(frame) })));
  }
}
