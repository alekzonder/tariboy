import { useCallback, useEffect, useState } from "react";
import { ApiError, type FileEntry, type FileListing } from "@/lib/api";
import { baseName, dirName } from "./paths";
import { errorText } from "@/lib/errors";

interface DirState {
  entries: FileEntry[] | null;
  loading: boolean;
  error: string | null;
}

export interface FileTreeApi {
  dirs: Record<string, DirState>;
  expanded: Set<string>;
  toggle: (path: string) => void;
  refresh: (path: string) => void;
  expand: (path: string) => void;
  // Whether `path` already exists, resolved against the live filesystem when
  // the parent listing is not cached. "unknown" means the parent could not be
  // listed (for a reason other than not-existing), so callers must not treat
  // the target as absent.
  resolveExists: (path: string) => Promise<"exists" | "absent" | "unknown">;
}

export function useFileTree(listDir: (path: string) => Promise<FileListing>): FileTreeApi {
  const [dirs, setDirs] = useState<Record<string, DirState>>({
    "": { entries: null, loading: false, error: null },
  });
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  // Fetch a directory. Safe to set `loading` synchronously here because this is
  // only ever called from event handlers (toggle/refresh), never an effect —
  // the mount fetch below sets state only from its async callbacks.
  const fetchDir = useCallback(
    (path: string) => {
      setDirs((prev) => ({
        ...prev,
        [path]: { entries: prev[path]?.entries ?? null, loading: true, error: null },
      }));
      void listDir(path)
        .then((r) =>
          setDirs((prev) => ({
            ...prev,
            [path]: { entries: r.entries, loading: false, error: null },
          })),
        )
        .catch((e) =>
          setDirs((prev) => ({
            ...prev,
            [path]: { entries: prev[path]?.entries ?? null, loading: false, error: errorText(e) },
          })),
        );
    },
    [listDir],
  );

  // Load the root on mount. The hook is remounted per source (the tree is keyed
  // by sourceKey in the parent), so the initial null state already reflects a
  // fresh source and this effect only sets state from async callbacks.
  useEffect(() => {
    let alive = true;
    void listDir("")
      .then((r) => {
        if (alive)
          setDirs((prev) => ({ ...prev, "": { entries: r.entries, loading: false, error: null } }));
      })
      .catch((e) => {
        if (alive)
          setDirs((prev) => ({
            ...prev,
            "": { entries: null, loading: false, error: errorText(e) },
          }));
      });
    return () => {
      alive = false;
    };
  }, [listDir]);

  const toggle = useCallback(
    (path: string) => {
      const isOpen = expanded.has(path);
      setExpanded((prev) => {
        const next = new Set(prev);
        if (isOpen) next.delete(path);
        else next.add(path);
        return next;
      });
      // First expansion of a not-yet-loaded directory triggers a fetch.
      if (!isOpen) {
        const st = dirs[path];
        if (st?.entries == null && !st?.loading) fetchDir(path);
      }
    },
    [expanded, dirs, fetchDir],
  );

  const refresh = useCallback(
    (path: string) => {
      fetchDir(path);
    },
    [fetchDir],
  );

  const expand = useCallback((path: string) => {
    setExpanded((prev) => (prev.has(path) ? prev : new Set(prev).add(path)));
  }, []);

  // Resolve whether `path` exists. When the destination's parent directory is
  // not cached we must not assume the target is absent — that would let a
  // rename silently clobber an existing file behind a collapsed folder. Fetch
  // the parent listing (and cache it) to decide; a missing parent means the
  // target cannot exist, while any other listing failure is indeterminate.
  const resolveExists = useCallback(
    async (path: string): Promise<"exists" | "absent" | "unknown"> => {
      const parent = dirName(path);
      const name = baseName(path);
      let entries = dirs[parent]?.entries ?? null;
      if (entries == null) {
        try {
          const r = await listDir(parent);
          entries = r.entries;
          setDirs((prev) => ({
            ...prev,
            [parent]: { entries: r.entries, loading: false, error: null },
          }));
        } catch (e) {
          if (e instanceof ApiError && e.code === "not_found") return "absent";
          return "unknown";
        }
      }
      return entries.some((x) => x.name === name) ? "exists" : "absent";
    },
    [dirs, listDir],
  );

  return { dirs, expanded, toggle, refresh, expand, resolveExists };
}
