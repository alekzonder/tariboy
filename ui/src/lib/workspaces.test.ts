import { beforeEach, describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import {
  DEFAULT_WORKSPACE_ID,
  WORKSPACES_KEY,
  addHostToWorkspace,
  createWorkspace,
  deleteWorkspace,
  hostsIn,
  inWorkspace,
  removeHostFromWorkspace,
  renameWorkspace,
  resetWorkspacesForTest,
  selectWorkspace,
  useWorkspaces,
} from "./workspaces";

beforeEach(() => {
  localStorage.clear();
  resetWorkspacesForTest();
});

const LEGACY_KEY = "app:workspaces:v1";

function stored() {
  return JSON.parse(localStorage.getItem(WORKSPACES_KEY)!);
}

describe("workspaces", () => {
  it("starts with the built-in Default holding every host", () => {
    const { result } = renderHook(() => useWorkspaces());
    expect(result.current.workspaces).toEqual([
      { id: DEFAULT_WORKSPACE_ID, name: "Default", hosts: [] },
    ]);
    expect(result.current.active).toBe(DEFAULT_WORKSPACE_ID);
    expect(inWorkspace(result.current, "", DEFAULT_WORKSPACE_ID)).toBe(true);
    expect(inWorkspace(result.current, "any-host", DEFAULT_WORKSPACE_ID)).toBe(true);
  });

  it("creates, renames and adds a host, updating subscribers and storage", () => {
    const { result } = renderHook(() => useWorkspaces());
    let id = "";
    act(() => {
      id = createWorkspace("  Production  ")!;
    });
    expect(result.current.workspaces.map((w) => w.name)).toEqual(["Default", "Production"]);

    act(() => addHostToWorkspace("h1", id));
    expect(inWorkspace(result.current, "h1", id)).toBe(true);
    expect(inWorkspace(result.current, "h1", DEFAULT_WORKSPACE_ID)).toBe(false);
    expect(inWorkspace(result.current, "h2", DEFAULT_WORKSPACE_ID)).toBe(true);
    expect(inWorkspace(result.current, "h2", id)).toBe(false);

    act(() => renameWorkspace(id, "Prod"));
    expect(result.current.workspaces[1]).toEqual({ id, name: "Prod", hosts: ["h1"] });
    expect(stored()).toMatchObject({
      version: 2,
      workspaces: [
        { id: DEFAULT_WORKSPACE_ID, hosts: [] },
        { id, name: "Prod", hosts: ["h1"] },
      ],
    });
  });

  it("keeps one host in several workspaces and removes it from one only", () => {
    const { result } = renderHook(() => useWorkspaces());
    let a = "";
    let b = "";
    act(() => {
      a = createWorkspace("A")!;
      b = createWorkspace("B")!;
      addHostToWorkspace("h1", a);
      addHostToWorkspace("h1", b);
      addHostToWorkspace("h1", b);
    });
    expect(result.current.workspaces[2].hosts).toEqual(["h1"]);
    const hosts = [{ id: "h1" }, { id: "h2" }];
    expect(hostsIn(result.current, hosts, a)).toEqual([{ id: "h1" }]);
    expect(hostsIn(result.current, hosts, b)).toEqual([{ id: "h1" }]);
    expect(hostsIn(result.current, hosts, DEFAULT_WORKSPACE_ID)).toEqual([{ id: "h2" }]);

    act(() => removeHostFromWorkspace("h1", a));
    expect(inWorkspace(result.current, "h1", a)).toBe(false);
    expect(inWorkspace(result.current, "h1", b)).toBe(true);
    expect(inWorkspace(result.current, "h1", DEFAULT_WORKSPACE_ID)).toBe(false);

    // A host left in no workspace falls back to Default.
    act(() => removeHostFromWorkspace("h1", b));
    expect(inWorkspace(result.current, "h1", DEFAULT_WORKSPACE_ID)).toBe(true);
  });

  it("holds a host added in Default next to its other workspaces", () => {
    const { result } = renderHook(() => useWorkspaces());
    let id = "";
    act(() => {
      id = createWorkspace("Lab")!;
      addHostToWorkspace("h1", id);
      addHostToWorkspace("h1", DEFAULT_WORKSPACE_ID);
    });
    expect(inWorkspace(result.current, "h1", id)).toBe(true);
    expect(inWorkspace(result.current, "h1", DEFAULT_WORKSPACE_ID)).toBe(true);
  });

  it("ignores blank names and never renames or deletes Default", () => {
    const { result } = renderHook(() => useWorkspaces());
    act(() => {
      expect(createWorkspace("   ")).toBeUndefined();
    });
    act(() => renameWorkspace(DEFAULT_WORKSPACE_ID, "Mine"));
    act(() => deleteWorkspace(DEFAULT_WORKSPACE_ID));
    act(() => {
      renameWorkspace(createWorkspace("A")!, "  ");
    });
    expect(result.current.workspaces.map((w) => w.name)).toEqual(["Default", "A"]);
  });

  it("returns a deleted workspace's hosts to Default and leaves it if it was current", () => {
    const { result } = renderHook(() => useWorkspaces());
    act(() => {
      const id = createWorkspace("Lab")!;
      addHostToWorkspace("h1", id);
      selectWorkspace(id);
    });
    const id = result.current.active;
    expect(id).not.toBe(DEFAULT_WORKSPACE_ID);

    act(() => deleteWorkspace(id));
    expect(result.current.workspaces).toHaveLength(1);
    expect(result.current.active).toBe(DEFAULT_WORKSPACE_ID);
    expect(inWorkspace(result.current, "h1", DEFAULT_WORKSPACE_ID)).toBe(true);
  });

  it("keeps the current workspace across a reload", () => {
    let id = "";
    act(() => {
      id = createWorkspace("Lab")!;
      selectWorkspace(id);
    });
    resetWorkspacesForTest();
    const { result } = renderHook(() => useWorkspaces());
    expect(result.current.active).toBe(id);
  });

  it("migrates a version 1 record: each host keeps its one workspace", () => {
    localStorage.setItem(
      LEGACY_KEY,
      JSON.stringify({
        version: 1,
        workspaces: [
          { id: "w1", name: "Lab" },
          { id: "w2", name: "Prod" },
        ],
        hostWorkspace: { h1: "w1", h2: "w2", h3: "w1" },
        active: "w2",
      }),
    );
    const { result } = renderHook(() => useWorkspaces());
    expect(result.current.workspaces).toEqual([
      { id: DEFAULT_WORKSPACE_ID, name: "Default", hosts: [] },
      { id: "w1", name: "Lab", hosts: ["h1", "h3"] },
      { id: "w2", name: "Prod", hosts: ["h2"] },
    ]);
    expect(result.current.active).toBe("w2");
    expect(inWorkspace(result.current, "h4", DEFAULT_WORKSPACE_ID)).toBe(true);
  });

  it("falls back to Default on malformed storage or dangling references", () => {
    localStorage.setItem(WORKSPACES_KEY, "{not json");
    const broken = renderHook(() => useWorkspaces());
    expect(broken.result.current.workspaces).toHaveLength(1);
    broken.unmount();

    localStorage.setItem(
      LEGACY_KEY,
      JSON.stringify({
        version: 1,
        workspaces: [{ id: "w1", name: "Lab" }, { id: 7 }, { id: DEFAULT_WORKSPACE_ID, name: "x" }],
        hostWorkspace: { h1: "gone", h2: "w1", h3: 5 },
        active: "gone",
      }),
    );
    resetWorkspacesForTest();
    const { result } = renderHook(() => useWorkspaces());
    expect(result.current.workspaces).toEqual([
      { id: DEFAULT_WORKSPACE_ID, name: "Default", hosts: [] },
      { id: "w1", name: "Lab", hosts: ["h2"] },
    ]);
    expect(result.current.active).toBe(DEFAULT_WORKSPACE_ID);
    expect(inWorkspace(result.current, "h1", DEFAULT_WORKSPACE_ID)).toBe(true);
    expect(inWorkspace(result.current, "h2", "w1")).toBe(true);
  });

  it("drops malformed version 2 entries", () => {
    localStorage.setItem(
      WORKSPACES_KEY,
      JSON.stringify({
        version: 2,
        workspaces: [
          { id: DEFAULT_WORKSPACE_ID, name: "Renamed", hosts: ["h1", 3] },
          { id: "w1", name: "Lab", hosts: "h2" },
          { id: "", name: "Blank", hosts: [] },
        ],
        active: "w1",
      }),
    );
    const { result } = renderHook(() => useWorkspaces());
    expect(result.current.workspaces).toEqual([
      { id: DEFAULT_WORKSPACE_ID, name: "Default", hosts: ["h1"] },
      { id: "w1", name: "Lab", hosts: [] },
    ]);
    expect(result.current.active).toBe("w1");
  });
});
