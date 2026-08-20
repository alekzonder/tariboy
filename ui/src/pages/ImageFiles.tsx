import { useCallback, useRef } from "react";
import FileBrowser from "@/components/FileBrowser";
import { useImageContext } from "@/components/ImageLayout";
import {
  imageFileRead, imageFilesList,
  type FileContent, type FileEntry, type FileListing, type ImageFileEntry,
} from "@/lib/api";

// Read-only Files tab for an image. The backend serves a FLAT list of every
// packed tar member (full slash paths) plus a single-file read; the shared
// FileBrowser instead expects a per-directory listDir. This page bridges the
// two: it fetches the flat list once, caches it, and synthesizes each
// directory's direct children (including intermediate dirs the tar may omit)
// on demand. No write callbacks are passed and readOnly is set, so the browser
// renders with no edit/create/rename/delete affordances.
export default function ImageFiles() {
  const { ref, hostKey } = useImageContext();
  // Cache the flat member list per ref so repeated listDir calls (one per
  // expanded directory) don't re-hit the endpoint.
  const cache = useRef<{ key: string; entries: ImageFileEntry[] } | null>(null);
  const sourceKey = `${hostKey}:${ref}`;

  const loadAll = useCallback(async (): Promise<ImageFileEntry[]> => {
    if (cache.current?.key === sourceKey) return cache.current.entries;
    const r = await imageFilesList(ref);
    const entries = r.files ?? [];
    cache.current = { key: sourceKey, entries };
    return entries;
  }, [ref, sourceKey]);

  const listDir = useCallback(
    async (dir: string): Promise<FileListing> => {
      const all = await loadAll();
      const prefix = dir ? `${dir}/` : "";
      // Fold the flat list into this directory's direct children, deriving
      // intermediate directories from deeper paths when no explicit tar dir
      // entry exists.
      const seen = new Map<string, FileEntry>();
      for (const e of all) {
        if (!e.path.startsWith(prefix)) continue;
        const rest = e.path.slice(prefix.length);
        if (rest === "") continue;
        const slash = rest.indexOf("/");
        if (slash < 0) {
          seen.set(rest, { name: rest, isDir: e.is_dir, size: e.size, mtime: 0 });
        } else {
          const name = rest.slice(0, slash);
          if (!seen.has(name)) seen.set(name, { name, isDir: true, size: 0, mtime: 0 });
        }
      }
      const entries = [...seen.values()].sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        return a.name.localeCompare(b.name);
      });
      return { path: dir, entries };
    },
    [loadAll],
  );

  const readFile = useCallback(
    async (path: string): Promise<FileContent> => {
      const r = await imageFileRead(ref, path);
      return { path: r.path, kind: "text", content: r.content, size: r.content.length };
    },
    [ref],
  );

  return <FileBrowser sourceKey={sourceKey} listDir={listDir} readFile={readFile} readOnly />;
}
