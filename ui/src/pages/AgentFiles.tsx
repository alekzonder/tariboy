import { useCallback } from "react";
import { useAgentName } from "@/lib/agent";
import {
  agentFileCreate, agentFileDelete, agentFileGet, agentFileList,
  agentFileRename, agentFileSave,
} from "@/lib/api";
import FileBrowser from "@/components/FileBrowser";

// The agent Files tab is a fully-editable FileBrowser wired to the per-agent
// file APIs. The tree/viewer/dialog logic lives in the shared component; this
// page only binds the data source to the current agent's name. The identical
// browser runs read-only over image contents in the image detail page.
export default function AgentFiles() {
  const name = useAgentName();
  // Bind each op to the current agent name. These are stable per name, and the
  // browser is remounted per name (via sourceKey), so the tree resets cleanly
  // when the agent changes.
  const listDir = useCallback((path: string) => agentFileList(name ?? "", path), [name]);
  const readFile = useCallback((path: string) => agentFileGet(name ?? "", path), [name]);
  const writeFile = useCallback(
    (path: string, content: string) => agentFileSave(name ?? "", path, content).then(() => undefined),
    [name],
  );
  const createFile = useCallback(
    (path: string, kind: "file" | "dir") => agentFileCreate(name ?? "", path, kind).then(() => undefined),
    [name],
  );
  const renameFile = useCallback(
    (from: string, to: string) => agentFileRename(name ?? "", from, to).then(() => undefined),
    [name],
  );
  const deleteFile = useCallback(
    (path: string) => agentFileDelete(name ?? "", path).then(() => undefined),
    [name],
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
