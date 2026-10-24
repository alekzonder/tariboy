import { Button } from "@/components/ui/button";

// ConfigEmptyState explains why search or "Changed only" hides every block.
export function ConfigEmptyState({ query, onClear }: { query: string; onClear: () => void }) {
  return (
    <div className="rounded-lg border border-dashed p-6 text-center">
      <p className="text-sm font-medium">
        {query.trim() ? `No settings match “${query.trim()}”` : "No blocks have unsaved changes"}
      </p>
      <p className="mt-1 text-xs text-muted-foreground">
        Unsaved changes in hidden blocks are kept.
      </p>
      <Button variant="outline" size="sm" className="mt-3" onClick={onClear}>
        Clear search
      </Button>
    </div>
  );
}
