import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  defaultTaskColumnLayout,
  moveTaskColumn,
  PHONE_TASK_QUERY,
  readTaskColumnLayout,
  resetTaskColumns,
  setTaskColumnWidth,
  TASK_COLUMN_LAYOUT_KEY,
  useTaskColumns,
} from "./taskColumnLayout";

beforeEach(() => {
  localStorage.clear();
  resetTaskColumns();
  localStorage.clear();
});
afterEach(() => vi.restoreAllMocks());

const ids = (columns: { id: string }[]) => columns.map((column) => column.id);

describe("task column layout record", () => {
  it("defaults to today's table", () => {
    expect(defaultTaskColumnLayout()).toEqual({
      order: ["priority", "key", "task", "agent", "status", "duration", "updated"],
      widths: {
        priority: 30,
        key: 168,
        agent: 104,
        status: 148,
        duration: 74,
        updated: 76,
      },
    });
  });

  it("defaults when the record is missing, malformed or of another version", () => {
    for (const value of [
      null,
      "{",
      JSON.stringify({ schemaVersion: 2, order: [] }),
      JSON.stringify([]),
    ]) {
      if (value === null) localStorage.removeItem(TASK_COLUMN_LAYOUT_KEY);
      else localStorage.setItem(TASK_COLUMN_LAYOUT_KEY, value);
      expect(readTaskColumnLayout()).toEqual(defaultTaskColumnLayout());
    }
  });

  it("defaults when storage throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(readTaskColumnLayout()).toEqual(defaultTaskColumnLayout());
  });

  it("drops unknown columns, puts missing ones back at their default place and clamps widths", () => {
    localStorage.setItem(
      TASK_COLUMN_LAYOUT_KEY,
      JSON.stringify({
        schemaVersion: 1,
        order: [
          "status",
          "bogus",
          "priority",
          "key",
          "task",
          "status",
          "agent",
          // A column the table no longer has, such as the removed `server`.
          "server",
          "updated",
        ],
        widths: { key: 10, status: 9000, agent: "wide", updated: 90, server: 50 },
      }),
    );

    // `duration` is missing, so it comes back after `status`, its default neighbour.
    expect(readTaskColumnLayout()).toEqual({
      order: ["status", "duration", "priority", "key", "task", "agent", "updated"],
      widths: {
        priority: 30,
        key: 96,
        agent: 104,
        status: 480,
        duration: 74,
        updated: 90,
      },
    });
  });
});

describe("task column store", () => {
  it("persists width and order changes and reads them back after a reload", () => {
    setTaskColumnWidth("status", 150);
    moveTaskColumn("status", "task");

    expect(JSON.parse(localStorage.getItem(TASK_COLUMN_LAYOUT_KEY)!)).toEqual({
      schemaVersion: 1,
      order: ["priority", "key", "status", "task", "agent", "duration", "updated"],
      widths: { ...defaultTaskColumnLayout().widths, status: 150 },
    });
    expect(readTaskColumnLayout().widths.status).toBe(150);
  });

  it("clamps a width to the column's bounds", () => {
    setTaskColumnWidth("key", 20);
    setTaskColumnWidth("updated", 5000);
    expect(readTaskColumnLayout().widths).toMatchObject({ key: 96, updated: 480 });
  });

  it("moves a column to the place of another, either way", () => {
    moveTaskColumn("priority", "status");
    expect(readTaskColumnLayout().order).toEqual([
      "key",
      "task",
      "agent",
      "status",
      "priority",
      "duration",
      "updated",
    ]);
    moveTaskColumn("updated", "key");
    expect(readTaskColumnLayout().order).toEqual([
      "updated",
      "key",
      "task",
      "agent",
      "status",
      "priority",
      "duration",
    ]);
  });

  it("keeps working in memory when storage refuses to write", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    const { result } = renderHook(() => useTaskColumns("agent"));
    act(() => setTaskColumnWidth("status", 200));
    expect(result.current.find((column) => column.id === "status")?.width).toBe(200);
  });

  it("resets to the default layout", () => {
    moveTaskColumn("updated", "priority");
    setTaskColumnWidth("key", 300);
    resetTaskColumns();
    expect(readTaskColumnLayout()).toEqual(defaultTaskColumnLayout());
  });
});

describe("useTaskColumns", () => {
  it("gives each table its own columns in the shared order", () => {
    expect(ids(renderHook(() => useTaskColumns("agent")).result.current)).toEqual([
      "priority",
      "key",
      "task",
      "status",
      "duration",
      "updated",
    ]);
    expect(ids(renderHook(() => useTaskColumns("all")).result.current)).toEqual([
      "priority",
      "key",
      "task",
      "agent",
      "status",
      "updated",
    ]);
  });

  it("re-renders every reader when the layout changes", () => {
    const { result } = renderHook(() => useTaskColumns("all"));
    act(() => moveTaskColumn("status", "priority"));
    expect(ids(result.current)).toEqual(["status", "priority", "key", "task", "agent", "updated"]);
    expect(result.current.find((column) => column.id === "task")?.width).toBeUndefined();
  });

  it("shows a phone only the columns that name a task, without touching the saved layout", () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === PHONE_TASK_QUERY,
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    act(() => setTaskColumnWidth("key", 300));
    act(() => moveTaskColumn("status", "priority"));
    const saved = localStorage.getItem(TASK_COLUMN_LAYOUT_KEY);

    const { result } = renderHook(() => useTaskColumns("all"));
    expect(result.current).toEqual([
      { id: "status", width: 84 },
      { id: "key", width: 96 },
      { id: "task", width: undefined },
    ]);
    expect(localStorage.getItem(TASK_COLUMN_LAYOUT_KEY)).toBe(saved);
    vi.unstubAllGlobals();
  });
});
