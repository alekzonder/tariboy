import { readResult, type Transport } from "./client";

// ---- Shared server file upload ----
export interface PushResult {
  path: string;
  abs: string;
  bytes: number;
}

// ---- File browser (CWD-jailed; singular /file namespace — /files is owned by
// cp downloads). Envelope error codes: bad_path / not_found / exists / is_dir /
// not_dir / bad_type. ----
export interface FileEntry {
  name: string;
  isDir: boolean;
  size: number;
  mtime: number;
}
export interface FileListing {
  path: string;
  entries: FileEntry[];
}
export interface FileContent {
  path: string;
  kind: "text" | "binary" | "too_large";
  content: string;
  size: number;
}

// ---- Filesystem browser (cwd path autocomplete; $HOME-jailed, dirs only) ----
// Distinct from the agent-CWD file browser: this lists directories under the
// daemon filesystem root (TARIBOY_FS_ROOT, default $HOME) so the create forms
// can pick an arbitrary cwd. Error codes: bad_path (403) / not_found (404) /
// not_dir (400).
export interface FsEntry {
  name: string;
  dir: boolean;
}
export interface FsListing {
  path: string;
  parent: string;
  entries: FsEntry[];
}

export function filesApi(t: Transport) {
  return {
    serverUploadFile: async (file: File): Promise<PushResult> => {
      if (file.size > 1024 * 1024 * 1024) throw new Error("File exceeds 1 GiB");
      const response = await t.raw(
        "PUT",
        `/api/files/raw?name=${encodeURIComponent(file.name)}`,
        file,
      );
      return readResult<PushResult>(response);
    },
    // Root listing uses an empty path. The path rides as a query param; the
    // daemon jails it under the agent's CWD.
    agentFileList: (name: string, path: string) =>
      t.agentGet<FileListing>(name, `file/list?path=${encodeURIComponent(path)}`),
    agentFileGet: (name: string, path: string) =>
      t.agentGet<FileContent>(name, `file?path=${encodeURIComponent(path)}`),
    // Writes. Save/create/rename take their path(s) in the JSON body; delete
    // takes the path in the query string (mirroring the read routes).
    agentFileSave: (name: string, path: string, content: string) =>
      t.agentPut<{ path: string; saved: boolean }>(name, "file", { path, content }),
    agentFileCreate: (name: string, path: string, type: "file" | "dir") =>
      t.agentPost<{ path: string; created: boolean }>(name, "file", { path, type }),
    agentFileRename: (name: string, from: string, to: string) =>
      t.agentPost<{ from: string; to: string; renamed: boolean }>(name, "file/rename", {
        from,
        to,
      }),
    agentFileDelete: (name: string, path: string) =>
      t.agentDelete<{ path: string; deleted: boolean }>(
        name,
        `file?path=${encodeURIComponent(path)}`,
      ),
    // fsList lists the subdirectories of `path` under the filesystem root. An
    // empty path lists the root itself and is omitted so the daemon applies its
    // root default.
    fsList: (path: string) =>
      t.get<FsListing>(`/api/fs/list${path ? `?path=${encodeURIComponent(path)}` : ""}`),
  };
}
export type FilesApi = ReturnType<typeof filesApi>;
