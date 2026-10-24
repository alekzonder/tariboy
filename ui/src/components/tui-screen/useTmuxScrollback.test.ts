import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TerminalSocketController } from "@/hooks/useTerminalSocket";
import { useTmuxScrollback } from "./useTmuxScrollback";

afterEach(cleanup);

type Props = { name: string; absent: boolean; status: TerminalSocketController["status"] };

function setup(initial: Partial<Props> = {}) {
  const send = vi.fn();
  const hook = renderHook((props: Props) => useTmuxScrollback({ ...props, send }), {
    initialProps: { name: "a1", absent: false, status: "open", ...initial },
  });
  return { send, ...hook };
}

describe("useTmuxScrollback", () => {
  it("enters scrollback by sending the tmux prefix and copy-mode key", () => {
    const { result, send } = setup();
    expect(result.current.copyModeRequested).toBe(false);
    act(() => result.current.enterScrollback());
    expect(send).toHaveBeenCalledWith("\x02[");
    expect(result.current.copyModeRequested).toBe(true);
    expect(result.current.exitAnnouncement).toBe("");
  });

  it("pages and exits by button, announcing the return", () => {
    const { result, send } = setup();
    act(() => result.current.enterScrollback());
    act(() => result.current.pageBack());
    act(() => result.current.pageForward());
    act(() => result.current.exitScrollback());
    expect(send.mock.calls.map((c) => c[0])).toEqual(["\x02[", "\x1b[5~", "\x1b[6~", "q"]);
    expect(result.current.copyModeRequested).toBe(false);
    expect(result.current.exitAnnouncement).toBe("Returned to live output");
  });

  it("clears scrollback on a typed q, announcing only when scrollback was showing", () => {
    const { result } = setup();
    act(() => result.current.onTerminalInput("q"));
    expect(result.current.exitAnnouncement).toBe("");
    act(() => result.current.enterScrollback());
    act(() => result.current.onTerminalInput("qq"));
    expect(result.current.copyModeRequested).toBe(true);
    act(() => result.current.onTerminalInput("q"));
    expect(result.current.copyModeRequested).toBe(false);
    expect(result.current.exitAnnouncement).toBe("Returned to live output");
  });

  it("keeps the terminal input handler stable across renders", () => {
    const { result, rerender } = setup();
    const first = result.current.onTerminalInput;
    act(() => result.current.enterScrollback());
    rerender({ name: "a2", absent: false, status: "open" });
    expect(result.current.onTerminalInput).toBe(first);
  });

  it("resets on a session change", () => {
    const { result, rerender } = setup();
    act(() => result.current.enterScrollback());
    act(() => result.current.exitScrollback());
    act(() => result.current.enterScrollback());
    rerender({ name: "a2", absent: false, status: "open" });
    expect(result.current.copyModeRequested).toBe(false);
    expect(result.current.exitAnnouncement).toBe("");
  });

  it("resets on a closed socket and sends nothing afterwards", () => {
    const { result, rerender, send } = setup();
    act(() => result.current.enterScrollback());
    rerender({ name: "a1", absent: false, status: "closed" });
    expect(result.current.copyModeRequested).toBe(false);
    act(() => result.current.onTerminalInput("q"));
    expect(result.current.exitAnnouncement).toBe("");
    send.mockClear();
    act(() => result.current.enterScrollback());
    expect(send).not.toHaveBeenCalled();
    expect(result.current.copyModeRequested).toBe(false);
  });
});
