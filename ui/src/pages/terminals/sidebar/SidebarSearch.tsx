import { Search } from "lucide-react";

/**
 * The search box at the top of the sidebar. The handoff draws it as a button
 * that opens a command palette; this app has no palette yet and a dead control
 * is worse than a live one, so it stays a real input that filters the list.
 * The ⌘K hint is the affordance the palette will inherit.
 */
export function SidebarSearch({ query, onQuery }: {
  query: string;
  onQuery: (value: string) => void;
}) {
  return (
    <label className="flex h-[30px] items-center gap-[7px] rounded-[8px] bg-muted px-[9px] text-muted-foreground">
      <Search className="size-[13px] shrink-0" aria-hidden="true" />
      <input
        type="search"
        value={query}
        onChange={(e) => onQuery(e.target.value)}
        aria-label="Search agents"
        placeholder="Search agents"
        className="min-w-0 flex-1 bg-transparent text-[12.5px] text-foreground outline-none placeholder:text-muted-foreground"
      />
      <span aria-hidden="true" className="shrink-0 text-[11px] opacity-60">⌘K</span>
    </label>
  );
}
