import {
  ChevronDown, ChevronRight, File as FileIcon, Folder, FolderOpen,
  Loader2, Pencil, Trash2,
} from "lucide-react";
import type { FileEntry } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { FileTreeApi } from "./useFileTree";
import { joinPath } from "./paths";

export function TreeRow({
  tree, path, entry, depth, selected, onSelect, onRename, onDelete,
}: {
  tree: FileTreeApi;
  path: string;
  entry: FileEntry;
  depth: number;
  selected: string | null;
  onSelect: (path: string) => void;
  // Omitted in read-only mode — the row then shows no rename/delete affordance.
  onRename?: (entry: FileEntry, path: string) => void;
  onDelete?: (entry: FileEntry, path: string) => void;
}) {
  const open = tree.expanded.has(path);
  const dir = tree.dirs[path];
  const indent = { paddingLeft: `${depth * 0.75 + 0.5}rem` };

  return (
    <div>
      <div
        className={cn(
          "group flex items-center gap-1 pr-1 text-sm hover:bg-accent",
          selected === path && "bg-accent font-medium",
        )}
      >
        <button
          type="button"
          onClick={() => (entry.isDir ? tree.toggle(path) : onSelect(path))}
          style={indent}
          className="flex min-w-0 flex-1 items-center gap-1 py-0.5 text-left"
        >
          {entry.isDir ? (
            <>
              {open ? <ChevronDown className="size-3.5 shrink-0" /> : <ChevronRight className="size-3.5 shrink-0" />}
              {open ? <FolderOpen className="size-3.5 shrink-0" /> : <Folder className="size-3.5 shrink-0" />}
            </>
          ) : (
            <>
              <span className="size-3.5 shrink-0" />
              <FileIcon className="size-3.5 shrink-0" />
            </>
          )}
          <span className="truncate">{entry.name}</span>
          {entry.isDir && dir?.loading && <Loader2 className="size-3.5 shrink-0 animate-spin" />}
        </button>
        {onRename && (
          <button
            type="button"
            aria-label={`Rename ${entry.name}`}
            onClick={() => onRename(entry, path)}
            className="shrink-0 rounded p-0.5 opacity-0 hover:bg-background focus-visible:opacity-100 group-hover:opacity-100"
          >
            <Pencil className="size-3.5" />
          </button>
        )}
        {onDelete && (
          <button
            type="button"
            aria-label={`Delete ${entry.name}`}
            onClick={() => onDelete(entry, path)}
            className="shrink-0 rounded p-0.5 opacity-0 hover:bg-background focus-visible:opacity-100 group-hover:opacity-100"
          >
            <Trash2 className="size-3.5" />
          </button>
        )}
      </div>
      {entry.isDir && open && dir?.entries && (
        <div>
          {dir.entries.length === 0 ? (
            <div style={{ paddingLeft: `${(depth + 1) * 0.75 + 0.5}rem` }} className="py-0.5 text-xs text-muted-foreground">
              empty
            </div>
          ) : (
            dir.entries.map((c) => (
              <TreeRow
                key={c.name}
                tree={tree}
                path={joinPath(path, c.name)}
                entry={c}
                depth={depth + 1}
                selected={selected}
                onSelect={onSelect}
                onRename={onRename}
                onDelete={onDelete}
              />
            ))
          )}
        </div>
      )}
    </div>
  );
}
