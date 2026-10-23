import { ApiError } from "@/lib/api";

// Join a directory path with a child name. The root path is "" (empty), so the
// first level is just the bare name — matching the backend's empty-path root.
export function joinPath(dir: string, name: string): string {
  return dir ? `${dir}/${name}` : name;
}

// The parent directory of a path ("" for a top-level entry, matching root).
export function dirName(path: string): string {
  const i = path.lastIndexOf("/");
  return i < 0 ? "" : path.slice(0, i);
}

export function baseName(path: string): string {
  const i = path.lastIndexOf("/");
  return i < 0 ? path : path.slice(i + 1);
}

export function errMsg(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}

export function fileExt(path: string): string {
  const base = path.split("/").pop() ?? path;
  const dot = base.lastIndexOf(".");
  return dot > 0 ? base.slice(dot + 1).toLowerCase() : "";
}

export function isMarkdown(path: string): boolean {
  const ext = fileExt(path);
  return ext === "md" || ext === "markdown";
}
