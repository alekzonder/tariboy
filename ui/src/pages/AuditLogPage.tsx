import { useAgentName } from "@/lib/agent";
import { FullAuditLog } from "@/components/FullAuditLog";
import { IterationAuditLog } from "@/components/IterationAuditLog";
import { IterationList } from "./audit-log/IterationList";
import { IterationTagEditor } from "./audit-log/IterationTagEditor";
import { useIterationList } from "./audit-log/useIterationList";
import { useIterationQuery } from "./audit-log/useIterationQuery";

// AuditLogPage is the merged Iterations + Audit Log view: one tab, a left column
// listing "Full log" plus every iteration (newest first), and a right pane that
// shows the full cross-iteration stream (default) or a single iteration's log.
// period, when given, is the Activity tab's shared window: it replaces the
// page's own started-at date filters.
export default function AuditLogPage({
  period,
}: { period?: { since: string; until: string } } = {}) {
  const name = useAgentName();
  const { tagFilter, afterFilter, beforeFilter, queryString, selected, select, setFilter } =
    useIterationQuery(period);
  const { items, loaded, setTags } = useIterationList(
    name,
    period ? tagFilter : queryString,
    queryString,
  );
  const selectedItem = selected ? items.find((it) => it.id === selected) : null;

  return (
    <div className="flex h-full gap-4">
      <IterationList
        items={items}
        selected={selected}
        select={select}
        tagFilter={tagFilter}
        afterFilter={afterFilter}
        beforeFilter={beforeFilter}
        showDates={!period}
        setFilter={setFilter}
      />
      <div className="min-h-0 flex-1 overflow-hidden">
        {selected === null ? (
          <FullAuditLog name={name} />
        ) : selectedItem ? (
          <div className="flex h-full flex-col">
            <IterationTagEditor name={name} item={selectedItem} onApplied={setTags} />
            <div className="min-h-0 flex-1">
              <IterationAuditLog
                name={name}
                iterationId={selected}
                iterationStatus={selectedItem.status}
                iterationProductive={selectedItem.productive}
              />
            </div>
          </div>
        ) : loaded ? (
          <p role="status" className="text-sm text-muted-foreground">
            Iteration {selected} was not found.
          </p>
        ) : (
          <p className="text-sm text-muted-foreground">Loading iteration…</p>
        )}
      </div>
    </div>
  );
}
