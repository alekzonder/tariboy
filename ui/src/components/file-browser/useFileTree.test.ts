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

it("expands, collapses and re-expands a directory from its cached listing", async () => {
  const listDir = vi.fn(async (path: string) => ({
    path,
    entries: [{ name: path ? "child" : "docs", isDir: !path, size: 1, mtime: 0 }],
  }));
  const { result } = renderHook(() => useFileTree(listDir));
  await waitFor(() => expect(result.current.dirs[""]?.entries).toHaveLength(1));

  act(() => result.current.toggle("docs"));
  expect(result.current.expanded.has("docs")).toBe(true);
  await waitFor(() => expect(result.current.dirs.docs?.entries?.[0].name).toBe("child"));
  expect(listDir).toHaveBeenLastCalledWith("docs");

  act(() => result.current.toggle("docs"));
  expect(result.current.expanded.has("docs")).toBe(false);
  expect(result.current.dirs.docs.entries?.[0].name).toBe("child");

  act(() => result.current.toggle("docs"));
  expect(result.current.expanded.has("docs")).toBe(true);
  expect(listDir).toHaveBeenCalledTimes(2);
});
