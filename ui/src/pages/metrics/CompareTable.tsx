import { ArrowDown, ArrowUp, Minus } from "lucide-react";
import { Unknown } from "@/components/metrics/Unknown";
import { UNKNOWN_REASON, versionLabel } from "@/lib/metricsFormat";
import { cn } from "@/lib/utils";
import { changeOf, fmtCell, type CompareRow } from "./compareModel";

const MISSING = "No data for this version in this period.";

function Value({ row, side }: { row: CompareRow; side: "a" | "b" }) {
  const text = fmtCell(row, side);
  return text ? <>{text}</> : <Unknown reason={MISSING} />;
}

/** The change and its verdict word; the icon's color repeats the word. */
function Change({ row }: { row: CompareRow }) {
  if (row.note) return null;
  const change = changeOf(row);
  if (!change) return <Unknown reason={UNKNOWN_REASON.change} />;
  const { verdict } = change;
  const Icon = verdict === "better" ? ArrowDown : verdict === "worse" ? ArrowUp : Minus;
  return (
    <>
      <span>{change.text}</span>
      {verdict && (
        <>
          {" "}
          <span className="inline-flex items-center gap-0.5 font-sans font-medium">
            <Icon
              aria-hidden="true"
              className={cn(
                "size-3",
                verdict === "better" && "text-status-running",
                verdict === "worse" && "text-destructive",
                verdict === "no change" && "text-muted-foreground",
              )}
            />
            {verdict}
          </span>
        </>
      )}
    </>
  );
}

/** The comparison: a table from 640 px, a list of blocks below it. */
export function CompareTable({ rows, a, b }: { rows: CompareRow[]; a: string; b: string }) {
  return (
    <>
      <div className="hidden overflow-x-auto rounded border sm:block">
        <table aria-label="Version comparison" className="w-full text-sm">
          <thead className="bg-muted text-left text-xs text-muted-foreground">
            <tr>
              <th scope="col" className="px-3 py-2 font-normal">
                Metric
              </th>
              <th scope="col" className="px-3 py-2 text-right font-normal">
                <span className="sr-only">Version A </span>
                {versionLabel(a)}
              </th>
              <th scope="col" className="px-3 py-2 text-right font-normal">
                <span className="sr-only">Version B </span>
                {versionLabel(b)}
              </th>
              <th scope="col" className="px-3 py-2 text-right font-normal">
                Change
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.id} className="border-t">
                <th scope="row" className="px-3 py-1.5 text-left font-normal">
                  {row.label}
                </th>
                <td className="px-3 py-1.5 text-right font-mono whitespace-nowrap">
                  <Value row={row} side="a" />
                </td>
                <td className="px-3 py-1.5 text-right font-mono whitespace-nowrap">
                  <Value row={row} side="b" />
                </td>
                <td className="px-3 py-1.5 text-right font-mono whitespace-nowrap">
                  <Change row={row} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ul aria-label="Version comparison list" className="space-y-2 sm:hidden">
        {rows.map((row) => (
          <li key={row.id} className="rounded border p-2 text-sm">
            <div className="font-medium">{row.label}</div>
            <div className="text-muted-foreground">
              A {versionLabel(a)}: <Value row={row} side="a" /> · B {versionLabel(b)}:{" "}
              <Value row={row} side="b" />
            </div>
            {!row.note && (
              <div>
                <Change row={row} />
              </div>
            )}
          </li>
        ))}
      </ul>
    </>
  );
}
