import { useCallback } from "react";
import { useAgentName } from "@/lib/agent";
import { useApi } from "@/components/apiContext";
import FileBrowser from "@/components/FileBrowser";

// The agent Files tab is a fully-editable FileBrowser wired to the per-agent
// file APIs. The tree/viewer/dialog logic lives in the shared component; this
// page only binds the data source to the current agent's name. The identical
// browser runs read-only over image contents in the image detail page.
export default function AgentFiles() {
  const api = useApi();
  const name = useAgentName();
  // Bind each op to the current agent name. These are stable per name, and the
  // browser is remounted per name (via sourceKey), so the tree resets cleanly
  // when the agent changes.
  const listDir = useCallback((path: string) => api.files.agentFileList(name ?? "", path), [api, name]);
  const readFile = useCallback((path: string) => api.files.agentFileGet(name ?? "", path), [api, name]);
  const writeFile = useCallback(
    (path: string, content: string) => api.files.agentFileSave(name ?? "", path, content).then(() => undefined),
    [api, name],
  );
  const createFile = useCallback(
    (path: string, kind: "file" | "dir") => api.files.agentFileCreate(name ?? "", path, kind).then(() => undefined),
    [api, name],
  );
  const renameFile = useCallback(
    (from: string, to: string) => api.files.agentFileRename(name ?? "", from, to).then(() => undefined),
    [api, name],
  );
  const deleteFile = useCallback(
    (path: string) => api.files.agentFileDelete(name ?? "", path).then(() => undefined),
    [api, name],
  );

  if (!name) return null;
  return (
    <FileBrowser
      sourceKey={name}
      listDir={listDir}
      readFile={readFile}
      writeFile={writeFile}
      createFile={createFile}
      renameFile={renameFile}
      deleteFile={deleteFile}
    />
  );
}
