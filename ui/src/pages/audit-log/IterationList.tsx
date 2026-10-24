import type { IterationSummary } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { fmtDateTime } from "@/lib/time";
import { cn } from "@/lib/utils";
import { statusVariant } from "./iterationStatus";

// IterationList is AuditLogPage's left column: the tag and started-at filters,
// "Full log", and every iteration (newest first).
export function IterationList({
  items,
  selected,
  select,
  tagFilter,
  afterFilter,
  beforeFilter,
  showDates,
  setFilter,
}: {
  items: IterationSummary[];
  selected: string | null;
  select: (iteration: string | null) => void;
  tagFilter: string;
  afterFilter: string;
  beforeFilter: string;
  showDates: boolean;
  setFilter: (key: string, value: string) => void;
}) {
  return (
    <div className="flex w-64 shrink-0 flex-col gap-2 overflow-auto">
      <div className="flex flex-col gap-1">
        <Input
          aria-label="Filter by tag"
          placeholder="tag, or tag,tag"
          value={tagFilter}
          onChange={(e) => setFilter("tag", e.target.value)}
        />
        {showDates && (
          <div className="flex gap-1">
            <Input
              aria-label="Started after"
              type="date"
              value={afterFilter}
              onChange={(e) => setFilter("started_after", e.target.value)}
            />
            <Input
              aria-label="Started before"
              type="date"
              value={beforeFilter}
              onChange={(e) => setFilter("started_before", e.target.value)}
            />
          </div>
        )}
      </div>
      <button
        onClick={() => select(null)}
        className={cn(
          "flex w-full items-center px-2 py-1.5 text-left text-sm hover:bg-accent",
          selected === null && "bg-accent",
        )}
      >
        Full log
      </button>
      {items.map((it) => (
        <button
          key={it.id}
          onClick={() => select(it.id)}
          className={cn(
            "flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left text-sm hover:bg-accent",
            selected === it.id && "bg-accent",
          )}
        >
          <span className="font-mono text-xs">{fmtDateTime(it.started_at)}</span>
          <span className="flex flex-wrap items-center justify-end gap-1">
            {(it.tags ?? []).map((tag) => (
              <Badge key={tag} variant="outline">
                {tag}
              </Badge>
            ))}
            {it.productive === false && (
              <Badge variant="outline" title="finished with i-am-done --idle (no productive work)">
                idle
              </Badge>
            )}
            <Badge variant={statusVariant(it.status)}>{it.status}</Badge>
          </span>
        </button>
      ))}
    </div>
  );
}
