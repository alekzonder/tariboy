import { Input } from "@/components/ui/input";
import { AuditExportActions } from "@/components/AuditExportActions";

const FILTER_DATALIST = "audit-type-presets";

// AuditFilterBar is FullAuditLog's filter row: the type input with its preset
// datalist, the full-text search, Clear while a filter is active, and export.
export function AuditFilterBar({
  name,
  type,
  query,
  presets,
  filtering,
  setFilter,
}: {
  name: string;
  type: string;
  query: string;
  presets: string[];
  filtering: boolean;
  setFilter: (type: string, query: string) => void;
}) {
  return (
    <div className="flex shrink-0 items-center gap-2">
      <Input
        aria-label="Filter by type"
        placeholder="type… (pick or type any)"
        list={FILTER_DATALIST}
        value={type}
        onChange={(e) => setFilter(e.target.value, query)}
        className="h-8 w-56"
      />
      <datalist id={FILTER_DATALIST}>
        {presets.map((t) => (
          <option key={t} value={t} />
        ))}
      </datalist>
      <Input
        aria-label="Search text"
        placeholder="search all fields…"
        value={query}
        onChange={(e) => setFilter(type, e.target.value)}
        className="h-8 flex-1"
      />
      {filtering && (
        <button
          onClick={() => setFilter("", "")}
          className="shrink-0 rounded border px-2 py-1 text-xs hover:bg-accent"
        >
          Clear
        </button>
      )}
      <AuditExportActions name={name} />
    </div>
  );
}
