import type {
  AgentFileContent,
  AgentFileCreated,
  AgentFileDeleted,
  AgentFileList,
  AgentFileRenamed,
  AgentFileSaved,
  FileUploadResult,
  FsListing,
} from "../api-types.generated";
import { readResult, type Transport } from "./client";

export type {
  AgentFileContent,
  AgentFileEntry,
  AgentFileList,
  FileUploadResult,
  FsEntry,
  FsListing,
} from "../api-types.generated";

export function filesApi(t: Transport) {
  return {
    serverUploadFile: async (file: File): Promise<FileUploadResult> => {
      if (file.size > 1024 * 1024 * 1024) throw new Error("File exceeds 1 GiB");
      const response = await t.raw(
        "PUT",
        `/api/files/raw?name=${encodeURIComponent(file.name)}`,
        file,
      );
      // PUT /api/files/raw is served by internal/api/server.go; its result is
      // the FileUploadResult of PUT /api/files.
      return readResult<FileUploadResult>(response);
    },
    // ---- File browser (CWD-jailed; singular /file namespace — /files is owned by
    // cp downloads). Envelope error codes: bad_path / not_found / exists / is_dir /
    // not_dir / bad_type. ----
    // Root listing uses an empty path. The path rides as a query param; the
    // daemon jails it under the agent's CWD.
    agentFileList: (name: string, path: string) =>
      t.agentGet<AgentFileList>(name, `file/list?path=${encodeURIComponent(path)}`),
    agentFileGet: (name: string, path: string) =>
      t.agentGet<AgentFileContent>(name, `file?path=${encodeURIComponent(path)}`),
    // Writes. Save/create/rename take their path(s) in the JSON body; delete
    // takes the path in the query string (mirroring the read routes).
    agentFileSave: (name: string, path: string, content: string) =>
      t.agentPut<AgentFileSaved>(name, "file", { path, content }),
    agentFileCreate: (name: string, path: string, type: "file" | "dir") =>
      t.agentPost<AgentFileCreated>(name, "file", { path, type }),
    agentFileRename: (name: string, from: string, to: string) =>
      t.agentPost<AgentFileRenamed>(name, "file/rename", {
        from,
        to,
      }),
    agentFileDelete: (name: string, path: string) =>
      t.agentDelete<AgentFileDeleted>(name, `file?path=${encodeURIComponent(path)}`),
    // ---- Filesystem browser (cwd path autocomplete; $HOME-jailed, dirs only) ----
    // Distinct from the agent-CWD file browser: this lists directories under the
    // daemon filesystem root (TARIBOY_FS_ROOT, default $HOME) so the create forms
    // can pick an arbitrary cwd. Error codes: bad_path (403) / not_found (404) /
    // not_dir (400).
    // fsList lists the subdirectories of `path` under the filesystem root. An
    // empty path lists the root itself and is omitted so the daemon applies its
    // root default.
    fsList: (path: string) =>
      t.get<FsListing>(`/api/fs/list${path ? `?path=${encodeURIComponent(path)}` : ""}`),
  };
}
export type FilesApi = ReturnType<typeof filesApi>;
