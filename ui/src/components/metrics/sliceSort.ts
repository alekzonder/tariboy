import { useState } from "react";
import type { ReactNode } from "react";

/** One column: its sort value (null sorts last either way) and its cell. */
export interface Column<R> {
  id: string;
  label: string;
  /** A hint on the header. */
  hint?: string;
  value: (row: R) => number | string | null;
  cell: (row: R) => ReactNode;
}

export interface Sort {
  id: string;
  dir: "asc" | "desc";
}

export function useSort(initial: Sort) {
  const [sort, setSort] = useState(initial);
  // A new column sorts high to low first; the same column turns around.
  const toggle = (id: string) =>
    setSort((prev) =>
      prev.id === id ? { id, dir: prev.dir === "desc" ? "asc" : "desc" } : { id, dir: "desc" },
    );
  return { sort, setSort, toggle };
}

function compare(a: number | string, b: number | string): number {
  if (typeof a === "number" && typeof b === "number") return a - b;
  return String(a).localeCompare(String(b), "en", { numeric: true });
}

export function sortRows<R>(rows: R[], columns: Column<R>[], sort: Sort): R[] {
  const column = columns.find((c) => c.id === sort.id);
  if (!column) return rows;
  const sign = sort.dir === "asc" ? 1 : -1;
  return [...rows].sort((x, y) => {
    const a = column.value(x);
    const b = column.value(y);
    if (a === null && b === null) return 0;
    if (a === null) return 1;
    if (b === null) return -1;
    return sign * compare(a, b);
  });
}
