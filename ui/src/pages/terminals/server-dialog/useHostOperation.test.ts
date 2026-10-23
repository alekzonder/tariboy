import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as desktop from "@/lib/desktop";
import type { DaemonMeta } from "@/lib/daemons";
import { useHostOperation } from "./useHostOperation";

afterEach(() => vi.restoreAllMocks());

describe("useHostOperation", () => {
  it("ignores output from another operation and removes both subscriptions", () => {
    let output!: (event: desktop.HostOutputEvent) => void;
    const offState = vi.fn();
    const offOutput = vi.fn();
    vi.spyOn(desktop, "onHostState").mockReturnValue(offState);
    vi.spyOn(desktop, "onHostProvisionOutput").mockImplementation((listener) => {
      output = listener;
      return offOutput;
    });
    const server: DaemonMeta = { id: "host-1", label: "Host", baseURL: "", kind: "ssh", state: "ready" };
    const { result, unmount } = renderHook(() => useHostOperation(true, "ssh", server, () => {}));
    act(() => {
      result.current.beginOperation("update");
      result.current.rememberOperationId("current");
    });
    act(() => output({ host_id: "host-1", operation_id: "other", stream: "stderr", text: "unrelated failure" } as desktop.HostOutputEvent));
    expect(result.current.output).toEqual([]);
    expect(result.current.operation.status).toBe("running");
    unmount();
    expect(offState).toHaveBeenCalledOnce();
    expect(offOutput).toHaveBeenCalledOnce();
  });
});
