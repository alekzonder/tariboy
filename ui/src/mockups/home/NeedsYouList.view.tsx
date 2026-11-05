import { ChevronRight, Mail } from "lucide-react";
import { Button } from "@/components/ui/button";

// A local view part of the Home mockup: the markup of the rows of
// `pages/home/NeedsYouList.tsx`, which is a container (it reads the Needs you
// context and opens the task drawer). handoff.md asks to split it.

/** One row, already formatted by the container. */
export interface NeedsYouViewRow {
  id: string;
  kind: "task" | "letter";
  /** The task key, or the sender of a letter. */
  label: string;
  /** The task title, or the letter subject. */
  title: string;
  /** "wait_customer · builder · 09:42", "letter · 09:15". */
  meta: string;
}

// Below `sm` the row is a grid: the key and the title on the first line, the
// meta under the title. From `sm` up it is one line and the meta takes at
// most half of the space after the key.
const ROW_CLASS =
  "grid w-full min-w-0 grid-cols-[6rem_minmax(0,1fr)] items-baseline gap-x-3 rounded-control px-2 py-1.5 text-left text-body-sm hover:bg-accent sm:flex";

export function NeedsYouListView({ rows }: { rows: NeedsYouViewRow[] }) {
  if (rows.length === 0) {
    return <p className="px-2 text-body-sm text-muted-foreground">Nothing needs you.</p>;
  }
  return (
    <ul className="space-y-0.5">
      {rows.map((row) => (
        <li key={row.id}>
          <div className="flex min-w-0 items-center gap-0.5">
            {row.kind === "task" ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-6 shrink-0"
                aria-label="Answer in place"
                aria-expanded={false}
              >
                <ChevronRight aria-hidden="true" className="size-3.5" />
              </Button>
            ) : (
              <span className="size-6 shrink-0" />
            )}
            <button type="button" className={ROW_CLASS}>
              {row.kind === "task" ? (
                <span className="w-24 shrink-0 truncate font-mono text-caption">{row.label}</span>
              ) : (
                <span className="flex w-24 shrink-0 items-center gap-1 text-caption text-muted-foreground">
                  <Mail className="size-3" aria-hidden="true" />
                  {row.label}
                </span>
              )}
              <span className="min-w-0 flex-1 truncate">{row.title}</span>
              <span className="col-start-2 min-w-0 truncate text-caption text-muted-foreground sm:max-w-1/2 sm:shrink-0">
                {row.meta}
              </span>
            </button>
          </div>
        </li>
      ))}
    </ul>
  );
}
