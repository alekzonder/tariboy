import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { ChevronRight, Mail } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { NeedsYouViewRow } from "./needsYouModel";

export interface NeedsYouListViewProps {
  rows: NeedsYouViewRow[];
  /** The ids of the task rows that show their card. */
  expanded: ReadonlySet<string>;
  onToggle: (id: string) => void;
  onOpen: (row: NeedsYouViewRow) => void;
  /** The "Answer in place" card of an expanded task row. */
  renderCard?: (row: NeedsYouViewRow) => ReactNode;
  /** "Show all (N)" under the rows, or null when every row is shown. */
  showAll: { label: string; href: string } | null;
}

// Below `sm` the row is a grid: the key and the title on the first line, the
// meta under the title. From `sm` up it is one line and the meta takes at
// most half of the space after the key.
const ROW_CLASS =
  "grid w-full min-w-0 grid-cols-[6rem_minmax(0,1fr)] items-baseline gap-x-3 rounded-control px-2 py-1.5 text-left text-body-sm hover:bg-accent sm:flex";

/** The rows of Needs you, already formatted by the container. */
export function NeedsYouListView({
  rows,
  expanded,
  onToggle,
  onOpen,
  renderCard,
  showAll,
}: NeedsYouListViewProps) {
  return (
    <>
      <ul className="space-y-0.5">
        {rows.map((row) => {
          const open = expanded.has(row.id);
          return (
            <li key={row.id}>
              <div className="flex min-w-0 items-center gap-0.5">
                {row.kind === "task" ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-6 shrink-0"
                    aria-label="Answer in place"
                    aria-expanded={open}
                    onClick={() => onToggle(row.id)}
                  >
                    <ChevronRight
                      aria-hidden="true"
                      className={cn("size-3.5", open && "rotate-90")}
                    />
                  </Button>
                ) : (
                  <span className="size-6 shrink-0" />
                )}
                <button type="button" className={ROW_CLASS} onClick={() => onOpen(row)}>
                  {row.kind === "task" ? (
                    <span className="w-24 shrink-0 truncate font-mono text-caption">
                      {row.label}
                    </span>
                  ) : (
                    <span className="flex w-24 shrink-0 items-center gap-1 text-caption text-muted-foreground">
                      <Mail className="size-3 shrink-0" aria-hidden="true" />
                      <span className="min-w-0 truncate">{row.label}</span>
                    </span>
                  )}
                  <span className="min-w-0 flex-1 truncate">{row.title}</span>
                  <span className="col-start-2 min-w-0 truncate text-caption text-muted-foreground sm:max-w-1/2 sm:shrink-0">
                    {row.meta}
                  </span>
                </button>
              </div>
              {open && row.kind === "task" && (
                <div className="py-1.5 pl-6.5">{renderCard?.(row)}</div>
              )}
            </li>
          );
        })}
      </ul>
      {showAll && (
        <Link
          to={showAll.href}
          className="ml-8 inline-block rounded-control px-2 py-1 text-body-sm text-muted-foreground hover:text-foreground"
        >
          {showAll.label}
        </Link>
      )}
    </>
  );
}
