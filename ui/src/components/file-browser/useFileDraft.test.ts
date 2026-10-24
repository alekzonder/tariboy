import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import type { FileContent } from "@/lib/api";
import { useFileDraft } from "./useFileDraft";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const file: FileContent = { path: "a.md", kind: "text", content: "old", size: 3 };

it("loads the file, tracks the draft and folds a save into the file", async () => {
  const writeFile = vi.fn().mockResolvedValue(undefined);
  const onSaved = vi.fn();
  const readFile = async () => file;
  const { result } = renderHook(() => useFileDraft("a.md", readFile, writeFile, onSaved));
  expect(result.current.loading).toBe(true);
  await waitFor(() => expect(result.current.file).toEqual(file));
  expect(result.current.loading).toBe(false);

  act(() => result.current.startEdit());
  expect(result.current).toMatchObject({ editing: true, draft: "old", preview: false });
  act(() => result.current.setDraft("new"));
  expect(result.current.dirty).toBe(true);

  act(() => result.current.save());
  await waitFor(() => expect(result.current.saving).toBe(false));
  expect(onSaved).toHaveBeenCalledWith("a.md");
  expect(writeFile).toHaveBeenCalledWith("a.md", "new");
  expect(result.current.file?.content).toBe("new");
  expect(result.current).toMatchObject({ dirty: false, saving: false, editing: true });

  act(() => result.current.cancelEdit());
  expect(result.current.editing).toBe(false);
});

it("starts idle without a path and reports read errors", async () => {
  const denied = () => Promise.reject(new Error("denied"));
  const noop = () => {};
  const idle = renderHook(() => useFileDraft(null, denied, undefined, noop));
  expect(idle.result.current.loading).toBe(false);

  const { result } = renderHook(() => useFileDraft("a.md", denied, undefined, noop));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.file).toBeNull();
  expect(toast.error).toHaveBeenCalledWith("denied");
});
