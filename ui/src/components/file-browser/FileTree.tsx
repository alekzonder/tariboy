import type { ReactNode } from "react";
import { Loader2 } from "lucide-react";
import type { FileEntry } from "@/lib/api";
import type { FileTreeApi } from "./useFileTree";
import { TreeRow } from "./TreeRow";

export function FileTree({
  tree,
  selected,
  onSelect,
  onRename,
  onDelete,
}: {
  tree: FileTreeApi;
  selected: string | null;
  onSelect: (path: string) => void;
  onRename?: (entry: FileEntry, path: string) => void;
  onDelete?: (entry: FileEntry, path: string) => void;
}) {
  const root = tree.dirs[""];
  let rootBody: ReactNode;
  if (root?.error && !root.entries) {
    rootBody = <div className="p-3 text-xs text-muted-foreground">{root.error}</div>;
  } else if (root?.entries == null) {
    rootBody = (
      <div className="flex items-center gap-2 p-3 text-xs text-muted-foreground">
        <Loader2 className="size-3.5 animate-spin" /> Loading…
      </div>
    );
  } else if (root.entries.length === 0) {
    rootBody = <div className="p-3 text-xs text-muted-foreground">Working directory is empty</div>;
  } else {
    rootBody = (
      <div className="py-1">
        {root.entries.map((e) => (
          <TreeRow
            key={e.name}
            tree={tree}
            path={e.name}
            entry={e}
            depth={0}
            selected={selected}
            onSelect={onSelect}
            onRename={onRename}
            onDelete={onDelete}
          />
        ))}
      </div>
    );
  }

  return <div className="min-h-0 flex-1 overflow-auto">{rootBody}</div>;
}
