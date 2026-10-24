import { Search } from "lucide-react";

/**
 * The search box at the top of the sidebar. The handoff draws it as a button
 * that opens a command palette; this app has no palette yet and a dead control
 * is worse than a live one, so it stays a real input that filters the list.
 * The ⌘K hint is the affordance the palette will inherit.
 */
export function SidebarSearch({
  query,
  onQuery,
}: {
  query: string;
  onQuery: (value: string) => void;
}) {
  return (
    <label className="flex h-7.5 items-center gap-1.75 rounded-md bg-muted px-2.25 text-muted-foreground">
      <Search className="size-3.25 shrink-0" aria-hidden="true" />
      <input
        type="search"
        value={query}
        onChange={(e) => onQuery(e.target.value)}
        aria-label="Search agents"
        placeholder="Search agents"
        className="min-w-0 flex-1 bg-transparent text-body-sm text-foreground outline-none placeholder:text-muted-foreground"
      />
      <span aria-hidden="true" className="shrink-0 text-caption opacity-60">
        ⌘K
      </span>
    </label>
  );
}
