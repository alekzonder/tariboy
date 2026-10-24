import { lazy, useState } from "react";
import type { FileContent, FileListing } from "@/lib/api";
import { useFileTree } from "./file-browser/useFileTree";
import { useFileActions } from "./file-browser/useFileActions";
import { FileTree } from "./file-browser/FileTree";
import { FileToolbar } from "./file-browser/FileToolbar";
import { FileBrowserDialogs } from "./file-browser/FileBrowserDialogs";
import { LazyBoundary } from "./LazyBoundary";

// CodeMirror, its languages and the highlighted preview load with the viewer.
const FileViewer = lazy(() =>
  import("./file-browser/FileViewer").then((m) => ({ default: m.FileViewer })),
);

// ---- Injected data source ---------------------------------------------------
// FileBrowser is source-agnostic: the caller supplies the read (and optionally
// write/manage) operations, and the browser renders the tree + viewer over
// them. Each write/manage callback enables its own action; readOnly disables
// all mutations. This is the ONE component
// shared by the agent Files tab (fully editable) and the image detail Files tab
// (read-only).
export interface FileBrowserApi {
  listDir: (path: string) => Promise<FileListing>;
  readFile: (path: string) => Promise<FileContent>;
  // Presence of these enables the matching affordance (unless readOnly is set).
  writeFile?: (path: string, content: string) => Promise<void>;
  createFile?: (path: string, kind: "file" | "dir") => Promise<void>;
  renameFile?: (from: string, to: string) => Promise<void>;
  deleteFile?: (path: string) => Promise<void>;
}

export interface FileBrowserProps extends FileBrowserApi {
  // Stable identity for the source (agent name / image ref). The whole tree
  // model resets when it changes, so callers do not need to key the element.
  sourceKey: string;
  // Force read-only regardless of which write/manage callbacks were passed.
  readOnly?: boolean;
}

function FileBrowserInner(props: FileBrowserProps) {
  const { listDir, readFile, writeFile, createFile, renameFile, deleteFile, readOnly } = props;
  const tree = useFileTree(listDir);
  const [selected, setSelected] = useState<string | null>(null);
  const actions = useFileActions(props, tree, selected, setSelected);
  // Each affordance requires both its callback AND write access. readOnly forces
  // everything off regardless of which callbacks were supplied.
  const canEdit = !readOnly && !!writeFile;
  const canCreate = !readOnly && !!createFile;
  const canRename = !readOnly && !!renameFile;
  const canDelete = !readOnly && !!deleteFile;

  return (
    <div className="flex h-full min-h-0 gap-0 overflow-hidden rounded border">
      <aside className="flex w-64 shrink-0 flex-col overflow-hidden border-r">
        <FileToolbar canCreate={canCreate} onNew={actions.openNew} />
        <FileTree
          tree={tree}
          selected={selected}
          onSelect={setSelected}
          onRename={canRename ? actions.onRename : undefined}
          onDelete={canDelete ? actions.onDelete : undefined}
        />
      </aside>
      <div className="min-w-0 flex-1 overflow-hidden">
        <LazyBoundary resetKey={selected ?? ""}>
          <FileViewer
            key={selected ?? "__none"}
            path={selected}
            readFile={readFile}
            writeFile={canEdit ? writeFile : undefined}
            onSaved={actions.refreshParent}
          />
        </LazyBoundary>
      </div>
      <FileBrowserDialogs actions={actions} canCreate={canCreate} canRename={canRename} />
    </div>
  );
}

// Keyed by sourceKey so the whole tree model resets when the source changes.
export default function FileBrowser(props: FileBrowserProps) {
  return <FileBrowserInner key={props.sourceKey} {...props} />;
}
