import { beforeEach, describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import {
  DEFAULT_WORKSPACE_ID,
  WORKSPACES_KEY,
  createWorkspace,
  deleteWorkspace,
  moveHost,
  renameWorkspace,
  resetWorkspacesForTest,
  selectWorkspace,
  useWorkspaces,
  workspaceOf,
} from "./workspaces";

beforeEach(() => {
  localStorage.clear();
  resetWorkspacesForTest();
});

function stored() {
  return JSON.parse(localStorage.getItem(WORKSPACES_KEY)!);
}

describe("workspaces", () => {
  it("starts with the built-in Default holding every host", () => {
    const { result } = renderHook(() => useWorkspaces());
    expect(result.current.workspaces).toEqual([{ id: DEFAULT_WORKSPACE_ID, name: "Default" }]);
    expect(result.current.active).toBe(DEFAULT_WORKSPACE_ID);
    expect(workspaceOf(result.current, "")).toBe(DEFAULT_WORKSPACE_ID);
    expect(workspaceOf(result.current, "any-host")).toBe(DEFAULT_WORKSPACE_ID);
  });

  it("creates, renames and moves a host, updating subscribers and storage", () => {
    const { result } = renderHook(() => useWorkspaces());
    let id = "";
    act(() => { id = createWorkspace("  Production  ")!; });
    expect(result.current.workspaces.map((w) => w.name)).toEqual(["Default", "Production"]);

    act(() => moveHost("h1", id));
    expect(workspaceOf(result.current, "h1")).toBe(id);
    expect(workspaceOf(result.current, "h2")).toBe(DEFAULT_WORKSPACE_ID);

    act(() => renameWorkspace(id, "Prod"));
    expect(result.current.workspaces[1]).toEqual({ id, name: "Prod" });
    expect(stored()).toMatchObject({
      version: 1,
      workspaces: [{ id, name: "Prod" }],
      hostWorkspace: { h1: id },
    });

    act(() => moveHost("h1", DEFAULT_WORKSPACE_ID));
    expect(stored().hostWorkspace).toEqual({});
  });

  it("ignores blank names and never renames or deletes Default", () => {
    const { result } = renderHook(() => useWorkspaces());
    act(() => { expect(createWorkspace("   ")).toBeUndefined(); });
    act(() => renameWorkspace(DEFAULT_WORKSPACE_ID, "Mine"));
    act(() => deleteWorkspace(DEFAULT_WORKSPACE_ID));
    act(() => { renameWorkspace(createWorkspace("A")!, "  "); });
    expect(result.current.workspaces.map((w) => w.name)).toEqual(["Default", "A"]);
  });

  it("returns a deleted workspace's hosts to Default and leaves it if it was current", () => {
    const { result } = renderHook(() => useWorkspaces());
    act(() => {
      const id = createWorkspace("Lab")!;
      moveHost("h1", id);
      selectWorkspace(id);
    });
    const id = result.current.active;
    expect(id).not.toBe(DEFAULT_WORKSPACE_ID);

    act(() => deleteWorkspace(id));
    expect(result.current.workspaces).toHaveLength(1);
    expect(result.current.active).toBe(DEFAULT_WORKSPACE_ID);
    expect(workspaceOf(result.current, "h1")).toBe(DEFAULT_WORKSPACE_ID);
  });

  it("keeps the current workspace across a reload", () => {
    let id = "";
    act(() => { id = createWorkspace("Lab")!; selectWorkspace(id); });
    resetWorkspacesForTest();
    const { result } = renderHook(() => useWorkspaces());
    expect(result.current.active).toBe(id);
  });

  it("falls back to Default on malformed storage or dangling references", () => {
    localStorage.setItem(WORKSPACES_KEY, "{not json");
    const broken = renderHook(() => useWorkspaces());
    expect(broken.result.current.workspaces).toHaveLength(1);
    broken.unmount();

    localStorage.setItem(WORKSPACES_KEY, JSON.stringify({
      version: 1,
      workspaces: [{ id: "w1", name: "Lab" }, { id: 7 }, { id: DEFAULT_WORKSPACE_ID, name: "x" }],
      hostWorkspace: { h1: "gone", h2: "w1", h3: 5 },
      active: "gone",
    }));
    resetWorkspacesForTest();
    const { result } = renderHook(() => useWorkspaces());
    expect(result.current.workspaces).toEqual([
      { id: DEFAULT_WORKSPACE_ID, name: "Default" },
      { id: "w1", name: "Lab" },
    ]);
    expect(result.current.active).toBe(DEFAULT_WORKSPACE_ID);
    expect(workspaceOf(result.current, "h1")).toBe(DEFAULT_WORKSPACE_ID);
    expect(workspaceOf(result.current, "h2")).toBe("w1");
  });
});
