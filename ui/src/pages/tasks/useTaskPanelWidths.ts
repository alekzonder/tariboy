import { useCallback, useRef, useState } from "react";
import { readKey, storageKeys, writeKey } from "@/lib/storage";

export const TASK_PANEL_WIDTHS_KEY = storageKeys.taskPanelWidths.name;
const TASK_PANEL_WIDTHS_SCHEMA_VERSION = 1 as const;

export const MIN_TASK_DETAIL_WIDTH = 320;

export type TaskPanelWidths = {
  detailWidth: number;
};

export function defaultTaskDetailWidth(): number {
  return clampTaskDetailWidth((globalThis.innerWidth || 820) / 2);
}

function defaults(): TaskPanelWidths {
  return { detailWidth: defaultTaskDetailWidth() };
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, Math.round(value)));
}

export function clampTaskDetailWidth(value: number): number {
  return clamp(
    value,
    MIN_TASK_DETAIL_WIDTH,
    Math.max(MIN_TASK_DETAIL_WIDTH, (globalThis.innerWidth || 820) - 80),
  );
}

export function readTaskPanelWidths(): TaskPanelWidths {
  const value = readKey(storageKeys.taskPanelWidths) as Record<string, unknown> | null;
  if (
    value?.schemaVersion !== TASK_PANEL_WIDTHS_SCHEMA_VERSION ||
    typeof value.detailWidth !== "number" ||
    !Number.isFinite(value.detailWidth)
  )
    return defaults();
  return { detailWidth: clampTaskDetailWidth(value.detailWidth) };
}

function persistTaskPanelWidths(widths: TaskPanelWidths): void {
  // Web Storage is a best-effort Desktop convenience.
  writeKey(storageKeys.taskPanelWidths, {
    schemaVersion: TASK_PANEL_WIDTHS_SCHEMA_VERSION,
    ...widths,
  });
}

export function useTaskPanelWidths(): TaskPanelWidths & {
  setDetailWidth: (value: number) => void;
} {
  const [widths, setWidths] = useState(readTaskPanelWidths);
  const widthsRef = useRef(widths);
  const update = useCallback((next: Partial<TaskPanelWidths>) => {
    const value = { ...widthsRef.current, ...next };
    widthsRef.current = value;
    setWidths(value);
    persistTaskPanelWidths(value);
  }, []);
  const setDetailWidth = useCallback(
    (value: number) => {
      update({ detailWidth: clampTaskDetailWidth(value) });
    },
    [update],
  );
  return { ...widths, setDetailWidth };
}
