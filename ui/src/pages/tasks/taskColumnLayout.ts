import { useSyncExternalStore } from "react";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { readKey, storageKeys, writeKey } from "@/lib/storage";
import { TASK_COLUMNS, type TaskRowMode } from "./taskColumns";

export const TASK_COLUMN_LAYOUT_KEY = storageKeys.taskColumns.name;
const TASK_COLUMN_LAYOUT_SCHEMA_VERSION = 1 as const;

/** Every column a task table can show, in the default order. `task` takes the
 *  rest of the row, so it can move but has no width of its own. */
const TASK_COLUMN_IDS = [
  "priority",
  "key",
  "task",
  "agent",
  "server",
  "status",
  "duration",
  "updated",
] as const;
export type TaskColumnId = (typeof TASK_COLUMN_IDS)[number];
export type SizedTaskColumnId = Exclude<TaskColumnId, "task">;

export const TASK_COLUMN_LABELS: Record<TaskColumnId, string> = {
  priority: "Pri",
  key: "Key",
  task: "Task",
  agent: "Agent",
  server: "Server",
  status: "Status",
  duration: "Duration",
  updated: "Updated",
};

const MODE_COLUMNS: Record<TaskRowMode, ReadonlySet<TaskColumnId>> = {
  agent: new Set(["priority", "key", "task", "status", "duration", "updated"]),
  all: new Set(["priority", "key", "task", "agent", "status", "updated"]),
  servers: new Set(["priority", "key", "task", "agent", "server", "status", "updated"]),
};

// `key` carries the tree indents and the chevron, so it cannot get narrow.
const MIN_WIDTH: Record<SizedTaskColumnId, number> = {
  priority: 24,
  key: 96,
  agent: 40,
  server: 40,
  status: 40,
  duration: 40,
  updated: 40,
};
const MAX_WIDTH = 480;

export type TaskColumnLayout = {
  order: TaskColumnId[];
  widths: Record<SizedTaskColumnId, number>;
};

export type TaskColumn = { id: TaskColumnId; width?: number };

export function defaultTaskColumnLayout(): TaskColumnLayout {
  const { priority, key, agent, server, status, duration, updated } = TASK_COLUMNS;
  return {
    order: [...TASK_COLUMN_IDS],
    widths: { priority, key, agent, server, status, duration, updated },
  };
}

export function clampTaskColumnWidth(id: SizedTaskColumnId, value: number): number {
  return Math.min(MAX_WIDTH, Math.max(MIN_WIDTH[id], Math.round(value)));
}

const isColumnId = (value: unknown): value is TaskColumnId =>
  typeof value === "string" && (TASK_COLUMN_IDS as readonly string[]).includes(value);

/** Keeps known columns once each, puts a missing one back after its default
 *  neighbour, and clamps every width; anything unreadable is the default. */
function normalize(value: unknown): TaskColumnLayout {
  const fallback = defaultTaskColumnLayout();
  if (!value || typeof value !== "object" || Array.isArray(value)) return fallback;
  const record = value as Record<string, unknown>;
  if (record.schemaVersion !== TASK_COLUMN_LAYOUT_SCHEMA_VERSION) return fallback;
  const order = [...new Set(Array.isArray(record.order) ? record.order.filter(isColumnId) : [])];
  TASK_COLUMN_IDS.forEach((id, index) => {
    if (order.includes(id)) return;
    const before = TASK_COLUMN_IDS.slice(0, index).findLast((previous) => order.includes(previous));
    order.splice(before ? order.indexOf(before) + 1 : 0, 0, id);
  });
  const stored =
    record.widths && typeof record.widths === "object"
      ? (record.widths as Record<string, unknown>)
      : {};
  const widths = { ...fallback.widths };
  for (const id of Object.keys(widths) as SizedTaskColumnId[]) {
    const width = stored[id];
    if (typeof width === "number" && Number.isFinite(width))
      widths[id] = clampTaskColumnWidth(id, width);
  }
  return { order, widths };
}

export function readTaskColumnLayout(): TaskColumnLayout {
  return normalize(readKey(storageKeys.taskColumns));
}

// One layout for every task table on the page, so the header, the rows and the
// Columns menu cannot disagree.
let current: TaskColumnLayout | undefined;
const listeners = new Set<() => void>();

function snapshot(): TaskColumnLayout {
  current ??= readTaskColumnLayout();
  return current;
}

function commit(next: TaskColumnLayout): void {
  current = next;
  // Web Storage is best effort: the change then lasts until a reload.
  writeKey(storageKeys.taskColumns, { schemaVersion: TASK_COLUMN_LAYOUT_SCHEMA_VERSION, ...next });
  listeners.forEach((listener) => listener());
}

export function setTaskColumnWidth(id: SizedTaskColumnId, width: number): void {
  const layout = snapshot();
  const next = clampTaskColumnWidth(id, width);
  if (layout.widths[id] === next) return;
  commit({ ...layout, widths: { ...layout.widths, [id]: next } });
}

/** Moves `id` to the place `target` holds, shifting the columns between. */
export function moveTaskColumn(id: TaskColumnId, target: TaskColumnId): void {
  const layout = snapshot();
  const from = layout.order.indexOf(id);
  const to = layout.order.indexOf(target);
  if (from === to) return;
  const order = [...layout.order];
  order.splice(from, 1);
  order.splice(to, 0, id);
  commit({ ...layout, order });
}

export function resetTaskColumns(): void {
  commit(defaultTaskColumnLayout());
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function columnsOf(layout: TaskColumnLayout, mode: TaskRowMode): TaskColumn[] {
  return layout.order
    .filter((id) => MODE_COLUMNS[mode].has(id))
    .map((id) => ({ id, width: id === "task" ? undefined : layout.widths[id] }));
}

function useTaskColumnLayout(): TaskColumnLayout {
  return useSyncExternalStore(subscribe, snapshot);
}

/** Below Tailwind's `sm` a task table keeps only what names a task. */
export const PHONE_TASK_QUERY = "(max-width: 639.98px)";
// Priority stays in the task's detail: with it the title got about 40px.
const PHONE_WIDTHS: Partial<Record<TaskColumnId, number>> = { key: 96, status: 84 };

/** The phone's columns at narrow fixed widths, in the saved order. The saved
 *  layout itself stays for wider screens. */
function phoneColumns(columns: TaskColumn[]): TaskColumn[] {
  return columns
    .filter((column) => column.id === "task" || column.id in PHONE_WIDTHS)
    .map((column) => ({ id: column.id, width: PHONE_WIDTHS[column.id] }));
}

/** The columns one table shows, in the shared order, with their widths. */
export function useTaskColumns(mode: TaskRowMode): TaskColumn[] {
  const columns = columnsOf(useTaskColumnLayout(), mode);
  return useMediaQuery(PHONE_TASK_QUERY) ? phoneColumns(columns) : columns;
}
