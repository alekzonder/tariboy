import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError, type ApiClient, type OnboardingStatus } from "@/lib/api";
import { useOnboardingStatus } from "./useOnboardingStatus";

const status = (store: string, starter: string): OnboardingStatus => ({
  agents: 0,
  default_store: { state: store, name: "tariboy", source: "s" },
  starter_image: { state: starter },
  harnesses: [],
});

const client = (getStatus: () => Promise<OnboardingStatus>, retry = vi.fn()) =>
  ({ onboarding: { getStatus, retry } }) as unknown as ApiClient;

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

it("polls every 5 seconds until every row is final", async () => {
  const getStatus = vi
    .fn()
    .mockResolvedValueOnce(status("pending", "pending"))
    .mockResolvedValueOnce(status("registered", "building"))
    .mockResolvedValue(status("registered", "ready"));
  const api = client(getStatus);
  const { result } = renderHook(() => useOnboardingStatus(api));
  await act(async () => {});
  expect(result.current.status?.default_store.state).toBe("pending");

  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000);
  });
  expect(result.current.status?.starter_image.state).toBe("building");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000);
  });
  expect(result.current.status?.starter_image.state).toBe("ready");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(20000);
  });
  expect(getStatus).toHaveBeenCalledTimes(3);
});

it("polls again after Retry", async () => {
  const getStatus = vi
    .fn()
    .mockResolvedValueOnce(status("failed", "pending"))
    .mockResolvedValue(status("registered", "ready"));
  const retry = vi.fn().mockResolvedValue(status("pending", "pending"));
  const api = client(getStatus, retry);
  const { result } = renderHook(() => useOnboardingStatus(api));
  await act(async () => {});
  await act(async () => {
    await vi.advanceTimersByTimeAsync(20000);
  });
  expect(getStatus).toHaveBeenCalledTimes(1);

  await act(async () => {
    await result.current.retry();
  });
  expect(retry).toHaveBeenCalledTimes(1);
  expect(result.current.status?.default_store.state).toBe("pending");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000);
  });
  expect(result.current.status?.default_store.state).toBe("registered");
});

it("gives no status for a daemon without the onboarding route", async () => {
  const getStatus = vi.fn().mockRejectedValue(new ApiError(404, "not_found", "no route"));
  const api = client(getStatus);
  const { result } = renderHook(() => useOnboardingStatus(api));
  await act(async () => {});
  expect(result.current.status).toBeNull();
  expect(result.current.loaded).toBe(true);
});

it("asks nothing without a client", async () => {
  const { result } = renderHook(() => useOnboardingStatus(null));
  await act(async () => {});
  expect(result.current.status).toBeNull();
  expect(result.current.loaded).toBe(false);
});
