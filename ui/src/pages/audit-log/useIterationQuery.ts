import { useSearchParams } from "react-router-dom";

// useIterationQuery reads AuditLogPage's URL state: the tag and started-at
// filters and the ?iteration= selection.
// period, when given, is the Activity tab's shared window: it replaces the
// page's own started-at date filters.
export function useIterationQuery(period?: { since: string; until: string }) {
  const [searchParams, setSearchParams] = useSearchParams();
  // Tag and started-at filters live in the URL beside ?iteration=, so a filtered
  // view is shareable and survives a reload. They are passed to the daemon,
  // which owns the matching (any listed tag; inclusive date bounds).
  const tagFilter = searchParams.get("tag") ?? "";
  const afterFilter = searchParams.get("started_after") ?? "";
  const beforeFilter = searchParams.get("started_before") ?? "";
  const query = new URLSearchParams();
  if (tagFilter) query.set("tag", tagFilter);
  const after = period ? period.since : afterFilter;
  const before = period ? period.until : beforeFilter;
  if (after) query.set("started_after", after);
  if (before) query.set("started_before", before);
  const queryString = query.toString();
  // selected iteration id; null = the Full log (default). Preselect from the
  // ?iteration= query param (e.g. an iteration row on the Usage tab links here);
  // absent param keeps the default Full-log pane.
  const selected = searchParams.get("iteration");
  const select = (iteration: string | null) => {
    const next = new URLSearchParams(searchParams);
    if (iteration) next.set("iteration", iteration);
    else next.delete("iteration");
    setSearchParams(next);
  };
  // A filter change replaces the history entry: typing into a filter must not
  // fill the back stack with one entry per keystroke.
  const setFilter = (key: string, value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value);
    else next.delete(key);
    setSearchParams(next, { replace: true });
  };
  return { tagFilter, afterFilter, beforeFilter, queryString, selected, select, setFilter };
}
