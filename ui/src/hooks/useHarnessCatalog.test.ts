import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, type ApiClient } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { fakeApi } from "@/test/api";
import { clearHarnessCatalogCache, useHarnessCatalog } from "./useHarnessCatalog";

const getHarnessModels = vi.fn();
const clientFor = (target: Daemon | null): ApiClient =>
  fakeApi({ harnesses: { getHarnessModels } }, target);

const remote: Daemon = { id: "remote", label: "Remote", baseURL: "http://r", token: "t" };
const catalog = {
  harness: "codex",
  models: [{ id: "gpt-6-astra", efforts: ["low"] }],
  efforts: ["low"],
  error: "",
};

beforeEach(() => {
  getHarnessModels.mockReset();
  clearHarnessCatalogCache();
});

describe("useHarnessCatalog", () => {
  it("loads the catalog from the selected host once per host and harness", async () => {
    getHarnessModels.mockResolvedValue(catalog);

    const remoteApi = clientFor(remote);
    const first = renderHook(() => useHarnessCatalog(remoteApi, "codex"));
    expect(first.result.current.loading).toBe(true);
    await waitFor(() => expect(first.result.current.catalog).toEqual(catalog));
    expect(getHarnessModels).toHaveBeenCalledWith("codex");

    const otherRemoteApi = clientFor(remote);
    const second = renderHook(() => useHarnessCatalog(otherRemoteApi, "codex"));
    expect(second.result.current).toEqual({ loading: false, catalog, error: "" });
    expect(getHarnessModels).toHaveBeenCalledTimes(1);
  });

  it("surfaces the host error from the catalog body", async () => {
    getHarnessModels.mockResolvedValue({
      ...catalog,
      models: [],
      error: "codex not found on PATH",
    });

    const localApi = clientFor(null);
    const { result } = renderHook(() => useHarnessCatalog(localApi, "codex"));
    await waitFor(() => expect(result.current.error).toBe("codex not found on PATH"));
  });

  it("reports a daemon without the route instead of failing", async () => {
    getHarnessModels.mockRejectedValue(new ApiError(404, "http_404", "HTTP 404"));

    const localApi = clientFor(null);
    const { result } = renderHook(() => useHarnessCatalog(localApi, "codex"));
    await waitFor(() =>
      expect(result.current.error).toBe("This host does not list harness models"),
    );
    expect(result.current.catalog).toBeNull();
  });

  it("does not ask without a harness", () => {
    const localApi = clientFor(null);
    const { result } = renderHook(() => useHarnessCatalog(localApi, ""));

    expect(result.current).toEqual({ loading: false, catalog: null, error: "" });
    expect(getHarnessModels).not.toHaveBeenCalled();
  });
});
