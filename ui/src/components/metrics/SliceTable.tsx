import { ArrowDown, ArrowUp } from "lucide-react";
import { cn } from "@/lib/utils";
import { sortRows, type Column, type Sort } from "./sliceSort";

const CELL = "px-3 py-1.5 whitespace-nowrap";
// The first column stays at the left edge while the table scrolls sideways.
const STICKY = "sticky left-0 z-10 bg-card";

/** A sortable table of slices in its own horizontal scroller. The first
 *  column, when `head` is given, is the row header and stays in place. */
export function SliceTable<R>({
  label,
  rows,
  rowKey,
  head,
  columns,
  sort,
  onSort,
  empty,
}: {
  label: string;
  rows: R[];
  rowKey: (row: R) => string;
  head?: Column<R>;
  columns: Column<R>[];
  sort: Sort;
  onSort: (id: string) => void;
  empty: string;
}) {
  const all = head ? [head, ...columns] : columns;
  const sorted = sortRows(rows, all, sort);
  const header = (column: Column<R>, first: boolean) => {
    const active = sort.id === column.id;
    const Arrow = sort.dir === "asc" ? ArrowUp : ArrowDown;
    return (
      <th
        key={column.id}
        scope="col"
        aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : undefined}
        className={cn(
          "px-3 py-2 font-normal whitespace-nowrap",
          first ? STICKY : "text-right",
          first && "bg-muted",
        )}
      >
        <button
          type="button"
          onClick={() => onSort(column.id)}
          className="inline-flex items-center gap-1 rounded hover:text-foreground"
        >
          <span title={column.hint}>{column.label}</span>
          {active && <Arrow aria-hidden="true" className="size-3" />}
        </button>
      </th>
    );
  };
  return (
    <div className="overflow-x-auto rounded border">
      <table aria-label={label} className="w-full text-sm">
        <thead className="bg-muted text-left text-xs text-muted-foreground">
          <tr>{all.map((column, i) => header(column, i === 0 && !!head))}</tr>
        </thead>
        <tbody>
          {sorted.map((row) => (
            <tr key={rowKey(row)} className="border-t">
              {head && (
                <th scope="row" className={cn(CELL, STICKY, "text-left font-normal")}>
                  {head.cell(row)}
                </th>
              )}
              {columns.map((column) => (
                <td key={column.id} className={cn(CELL, "text-right font-mono")}>
                  {column.cell(row)}
                </td>
              ))}
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={all.length} className="px-3 py-4 text-center text-muted-foreground">
                {empty}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

/** Below 640 px the headers can be off screen: a list picks the column and a
 *  button turns the direction. */
export function SortBySelect<R>({
  columns,
  sort,
  onChange,
}: {
  columns: Column<R>[];
  sort: Sort;
  onChange: (sort: Sort) => void;
}) {
  return (
    <div className="flex items-center gap-2 text-sm sm:hidden">
      <label className="flex items-center gap-1">
        <span className="text-muted-foreground">Sort by</span>
        <select
          value={sort.id}
          onChange={(e) => onChange({ id: e.target.value, dir: sort.dir })}
          className="h-7 rounded-control border bg-background px-2 text-body-sm"
        >
          {columns.map((c) => (
            <option key={c.id} value={c.id}>
              {c.label}
            </option>
          ))}
        </select>
      </label>
      <button
        type="button"
        onClick={() => onChange({ id: sort.id, dir: sort.dir === "asc" ? "desc" : "asc" })}
        className="h-7 rounded-control border px-2 text-body-sm"
      >
        {sort.dir === "asc" ? "Low to high" : "High to low"}
      </button>
    </div>
  );
}
