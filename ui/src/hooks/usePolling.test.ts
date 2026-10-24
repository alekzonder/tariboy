import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { usePolling } from "./usePolling";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

it("calls the latest fetcher on the next tick without restarting the timer", async () => {
  const first = vi.fn().mockResolvedValue("first");
  const second = vi.fn().mockResolvedValue("second");
  const { result, rerender } = renderHook(({ fn }) => usePolling(fn, 1000), {
    initialProps: { fn: first },
  });
  await act(async () => {});
  expect(first).toHaveBeenCalledTimes(1);
  expect(result.current.data).toBe("first");

  rerender({ fn: second });
  expect(second).not.toHaveBeenCalled();

  await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
  expect(first).toHaveBeenCalledTimes(1);
  expect(second).toHaveBeenCalledTimes(1);
  expect(result.current.data).toBe("second");
});
