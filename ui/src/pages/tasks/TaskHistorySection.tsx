import type { TaskEvent } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { SectionHeading } from "./TaskDetailControls";
import { EMPTY, QUIET_ACTION, ROW } from "./panelStyles";
import { formatTaskTime } from "./taskTime";

export function TaskHistorySection({
  events,
  historyOpen,
  setHistoryOpen,
}: {
  events: TaskEvent[];
  historyOpen: boolean;
  setHistoryOpen: (open: boolean) => void;
}) {
  return (
    <section className="flex min-w-0 flex-col gap-1">
      <SectionHeading
        label="History"
        count={events.length}
        action={
          <Button
            type="button"
            variant="ghost"
            className={QUIET_ACTION}
            aria-expanded={historyOpen}
            onClick={() => setHistoryOpen(!historyOpen)}
          >
            {historyOpen ? "Collapse" : "Expand"}
          </Button>
        }
      />
      {historyOpen &&
        events.map((event) => (
          <div key={event.event_id} className={ROW}>
            <time
              dateTime={event.created_at}
              title={new Date(event.created_at).toLocaleString()}
              className="w-13 shrink-0 font-mono text-caption tabular-nums text-muted-foreground"
            >
              {formatTaskTime(event.created_at)}
            </time>
            <span className="shrink-0 font-mono text-meta font-medium">{event.kind}</span>
            <span className="min-w-0 flex-1 truncate text-ui text-muted-foreground">
              {summarizePayload(event.payload)}
            </span>
            <span className="shrink-0 font-mono text-caption text-muted-foreground">
              {event.actor}
            </span>
          </div>
        ))}
      {historyOpen && events.length === 0 && (
        <span className={EMPTY}>Nothing has happened yet.</span>
      )}
    </section>
  );
}

/** Event payloads read as `key value · key value`; a JSON dump is a log, not a
 *  row, and the panel never shows one. */
function summarizePayload(payload: Record<string, unknown> | undefined): string {
  if (!payload) return "";
  return Object.entries(payload)
    .filter(([, value]) => value !== null && value !== undefined && value !== "")
    .map(
      ([key, value]) =>
        `${key} ${typeof value === "object" ? JSON.stringify(value) : String(value)}`,
    )
    .join(" · ");
}
