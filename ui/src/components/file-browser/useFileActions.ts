import { useCallback, useState, type Dispatch, type SetStateAction } from "react";
import { toast } from "sonner";
import type { AgentFileEntry } from "@/lib/api";
import type { FileBrowserApi } from "../FileBrowser";
import type { FileTreeApi } from "./useFileTree";
import { dirName } from "./paths";
import { errorText } from "@/lib/errors";

type NewKind = "file" | "dir";
interface RenameState {
  path: string;
  isDir: boolean;
}
interface ConfirmState {
  title: string;
  body: string;
  actionLabel: string;
  run: () => void;
}

export function useFileActions(
  { createFile, renameFile, deleteFile }: FileBrowserApi,
  tree: FileTreeApi,
  selected: string | null,
  setSelected: Dispatch<SetStateAction<string | null>>,
) {
  const [newKind, setNewKind] = useState<NewKind | null>(null);
  const [rename, setRename] = useState<RenameState | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  // Shared text for whichever path-prompt dialog is open; seeded by the opener.
  const [promptValue, setPromptValue] = useState("");

  const refreshParent = useCallback(
    (path: string) => {
      const parent = dirName(path);
      // Whether the immediate parent was already loaded decides how deep we must
      // refresh: a not-yet-loaded parent may have just been materialized (backend
      // MkdirAll) along with deeper intermediates.
      const parentLoaded = tree.dirs[parent]?.entries != null;
      tree.expand(parent);
      tree.refresh(parent);
      // Creating/renaming into a NEW nested path materializes intermediate
      // directories. The immediate-parent refresh above cannot reveal them —
      // walk up to the nearest already-loaded ancestor and refetch it so the new
      // directory becomes visible without a full reload.
      if (!parentLoaded) {
        let ancestor = dirName(parent);
        for (;;) {
          if (tree.dirs[ancestor]?.entries != null) {
            tree.refresh(ancestor);
            break;
          }
          if (ancestor === "") break;
          ancestor = dirName(ancestor);
        }
      }
    },
    [tree],
  );

  const doCreate = useCallback(
    (kind: NewKind, path: string) => {
      if (!createFile) return;
      void createFile(path, kind)
        .then(() => {
          toast.success(`Created ${path}`);
          refreshParent(path);
          setNewKind(null);
        })
        .catch((e) => toast.error(errorText(e)));
    },
    [createFile, refreshParent],
  );

  const doRename = useCallback(
    (from: string, to: string) => {
      if (!renameFile) return;
      void renameFile(from, to)
        .then(() => {
          toast.success(`Renamed to ${to}`);
          refreshParent(from);
          refreshParent(to);
          if (selected === from) setSelected(to);
          setRename(null);
        })
        .catch((e) => toast.error(errorText(e)));
    },
    [renameFile, refreshParent, selected, setSelected],
  );

  const submitRename = useCallback(
    (to: string) => {
      if (!rename) return;
      if (to === rename.path) {
        setRename(null);
        return;
      }
      const from = rename.path;
      // Rename overwrites silently on the backend (os.Rename), so guard against
      // clobbering the target. Resolve existence against the filesystem — even
      // when the destination's parent folder is collapsed/unloaded — and never
      // proceed without a confirm on a known-present or indeterminate target.
      void tree.resolveExists(to).then((state) => {
        if (state === "absent") {
          doRename(from, to);
          return;
        }
        setRename(null);
        setConfirm({
          title: "Overwrite?",
          body:
            state === "exists"
              ? `"${to}" already exists and will be overwritten.`
              : `Could not verify whether "${to}" already exists; renaming may overwrite it.`,
          actionLabel: "Overwrite",
          run: () => doRename(from, to),
        });
      });
    },
    [rename, tree, doRename],
  );

  const doDelete = useCallback(
    (path: string) => {
      if (!deleteFile) return;
      void deleteFile(path)
        .then(() => {
          toast.success(`Deleted ${path}`);
          refreshParent(path);
          if (selected === path) setSelected(null);
        })
        .catch((e) => toast.error(errorText(e)));
    },
    [deleteFile, refreshParent, selected, setSelected],
  );

  const onDelete = useCallback(
    (entry: AgentFileEntry, path: string) => {
      setConfirm({
        title: `Delete ${entry.isDir ? "folder" : "file"}?`,
        body: entry.isDir
          ? `"${path}" and everything inside it will be permanently deleted.`
          : `"${path}" will be permanently deleted.`,
        actionLabel: "Delete",
        run: () => doDelete(path),
      });
    },
    [doDelete],
  );

  const onRename = useCallback((entry: AgentFileEntry, path: string) => {
    setPromptValue(path);
    setRename({ path, isDir: entry.isDir });
  }, []);

  const openNew = useCallback((kind: NewKind) => {
    setPromptValue("");
    setNewKind(kind);
  }, []);

  return {
    newKind,
    setNewKind,
    rename,
    setRename,
    confirm,
    setConfirm,
    promptValue,
    setPromptValue,
    refreshParent,
    doCreate,
    submitRename,
    onDelete,
    onRename,
    openNew,
  };
}
