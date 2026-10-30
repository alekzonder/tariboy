import { SearchInput } from "@/components/ui/search-input";

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
    <SearchInput
      className="h-7.5"
      iconClassName="size-3.25"
      inputClassName="text-body-sm"
      value={query}
      onChange={(e) => onQuery(e.target.value)}
      aria-label="Search agents"
      placeholder="Search agents"
      hint="⌘K"
    />
  );
}
