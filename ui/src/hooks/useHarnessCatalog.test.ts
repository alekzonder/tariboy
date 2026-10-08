import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, getHarnessModelsOn } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { clearHarnessCatalogCache, useHarnessCatalog } from "./useHarnessCatalog";

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return { ...actual, getHarnessModelsOn: vi.fn() };
});

const remote: Daemon = { id: "remote", label: "Remote", baseURL: "http://r", token: "t" };
const catalog = {
  harness: "codex",
  models: [{ id: "gpt-6-astra", efforts: ["low"] }],
  efforts: ["low"],
  error: "",
};

beforeEach(() => {
  vi.mocked(getHarnessModelsOn).mockReset();
  clearHarnessCatalogCache();
});

describe("useHarnessCatalog", () => {
  it("loads the catalog from the selected host once per host and harness", async () => {
    vi.mocked(getHarnessModelsOn).mockResolvedValue(catalog);

    const first = renderHook(() => useHarnessCatalog(remote, "codex"));
    expect(first.result.current.loading).toBe(true);
    await waitFor(() => expect(first.result.current.catalog).toEqual(catalog));
    expect(getHarnessModelsOn).toHaveBeenCalledWith(remote, "codex");

    const second = renderHook(() => useHarnessCatalog(remote, "codex"));
    expect(second.result.current).toEqual({ loading: false, catalog, error: "" });
    expect(getHarnessModelsOn).toHaveBeenCalledTimes(1);
  });

  it("surfaces the host error from the catalog body", async () => {
    vi.mocked(getHarnessModelsOn).mockResolvedValue({
      ...catalog,
      models: [],
      error: "codex not found on PATH",
    });

    const { result } = renderHook(() => useHarnessCatalog(null, "codex"));
    await waitFor(() => expect(result.current.error).toBe("codex not found on PATH"));
  });

  it("reports a daemon without the route instead of failing", async () => {
    vi.mocked(getHarnessModelsOn).mockRejectedValue(
      new ApiError(404, "http_404", "HTTP 404"),
    );

    const { result } = renderHook(() => useHarnessCatalog(null, "codex"));
    await waitFor(() =>
      expect(result.current.error).toBe("This host does not list harness models"),
    );
    expect(result.current.catalog).toBeNull();
  });

  it("does not ask without a harness", () => {
    const { result } = renderHook(() => useHarnessCatalog(null, ""));

    expect(result.current).toEqual({ loading: false, catalog: null, error: "" });
    expect(getHarnessModelsOn).not.toHaveBeenCalled();
  });
});
