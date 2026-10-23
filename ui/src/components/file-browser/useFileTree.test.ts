import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useFileTree } from "./useFileTree";

afterEach(cleanup);

it("resolves overwrite targets from unloaded parents and caches the listing", async () => {
  const listDir = vi.fn(async (path: string) => ({
    path,
    entries: path === "closed" ? [{ name: "target", isDir: false, size: 1, mtime: 0 }] : [],
  }));
  const { result } = renderHook(() => useFileTree(listDir));
  await waitFor(() => expect(result.current.dirs[""]?.entries).toEqual([]));
  await act(async () => {
    expect(await result.current.resolveExists("closed/target")).toBe("exists");
  });
  expect(result.current.dirs.closed.entries?.[0].name).toBe("target");
  await act(async () => {
    expect(await result.current.resolveExists("closed/missing")).toBe("absent");
  });
  expect(listDir).toHaveBeenCalledTimes(2);
});
