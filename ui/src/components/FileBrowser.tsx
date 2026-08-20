import { useCallback, useEffect, useState, type ReactNode } from "react";
import { toast } from "sonner";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import CodeMirror from "@uiw/react-codemirror";
import { javascript } from "@codemirror/lang-javascript";
import { python } from "@codemirror/lang-python";
import { json } from "@codemirror/lang-json";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { markdown } from "@codemirror/lang-markdown";
import { go } from "@codemirror/lang-go";
import type { Extension } from "@codemirror/state";
import {
  ChevronDown, ChevronRight, File as FileIcon, FilePlus, Folder, FolderOpen,
  FolderPlus, Loader2, Pencil, Save, Trash2,
} from "lucide-react";
import { useTheme } from "@/components/theme-provider";
import { ApiError, type FileContent, type FileEntry, type FileListing } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";
import "@/styles/hljs-github.css";

// ---- Injected data source ---------------------------------------------------
// FileBrowser is source-agnostic: the caller supplies the read (and optionally
// write/manage) operations, and the browser renders the tree + viewer over
// them. Omitting writeFile — or passing readOnly — yields a read-only browser
// with no edit/create/rename/delete affordances. This is the ONE component
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

// Join a directory path with a child name. The root path is "" (empty), so the
// first level is just the bare name — matching the backend's empty-path root.
function joinPath(dir: string, name: string): string {
  return dir ? `${dir}/${name}` : name;
}

// The parent directory of a path ("" for a top-level entry, matching root).
function dirName(path: string): string {
  const i = path.lastIndexOf("/");
  return i < 0 ? "" : path.slice(0, i);
}

function baseName(path: string): string {
  const i = path.lastIndexOf("/");
  return i < 0 ? path : path.slice(i + 1);
}

function errMsg(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}

function fileExt(path: string): string {
  const base = path.split("/").pop() ?? path;
  const dot = base.lastIndexOf(".");
  return dot > 0 ? base.slice(dot + 1).toLowerCase() : "";
}

function isMarkdown(path: string): boolean {
  const ext = fileExt(path);
  return ext === "md" || ext === "markdown";
}

// Map a file extension to a CodeMirror language extension. A handful of common
// languages are wired; anything else falls back to a plain (unhighlighted)
// editor.
function langFor(path: string): Extension[] {
  switch (fileExt(path)) {
    case "js": case "jsx": case "mjs": case "cjs":
      return [javascript({ jsx: true })];
    case "ts":
      return [javascript({ typescript: true })];
    case "tsx":
      return [javascript({ jsx: true, typescript: true })];
    case "py":
      return [python()];
    case "json":
      return [json()];
    case "html": case "htm":
      return [html()];
    case "css":
      return [css()];
    case "md": case "markdown":
      return [markdown()];
    case "go":
      return [go()];
    default:
      return [];
  }
}

// ---- Lifted tree model ------------------------------------------------------
// Directory listings are held centrally (keyed by directory path) instead of
// each node owning its own cache, so a mutation can refetch exactly the
// affected directory without collapsing the rest of the tree.

interface DirState { entries: FileEntry[] | null; loading: boolean; error: string | null }

interface FileTreeApi {
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

function useFileTree(listDir: (path: string) => Promise<FileListing>): FileTreeApi {
  const [dirs, setDirs] = useState<Record<string, DirState>>({
    "": { entries: null, loading: false, error: null },
  });
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  // Fetch a directory. Safe to set `loading` synchronously here because this is
  // only ever called from event handlers (toggle/refresh), never an effect —
  // the mount fetch below sets state only from its async callbacks.
  const fetchDir = useCallback((path: string) => {
    setDirs((prev) => ({ ...prev, [path]: { entries: prev[path]?.entries ?? null, loading: true, error: null } }));
    void listDir(path)
      .then((r) => setDirs((prev) => ({ ...prev, [path]: { entries: r.entries, loading: false, error: null } })))
      .catch((e) => setDirs((prev) => ({
        ...prev, [path]: { entries: prev[path]?.entries ?? null, loading: false, error: errMsg(e) },
      })));
  }, [listDir]);

  // Load the root on mount. The hook is remounted per source (the tree is keyed
  // by sourceKey in the parent), so the initial null state already reflects a
  // fresh source and this effect only sets state from async callbacks.
  useEffect(() => {
    let alive = true;
    void listDir("")
      .then((r) => { if (alive) setDirs((prev) => ({ ...prev, "": { entries: r.entries, loading: false, error: null } })); })
      .catch((e) => { if (alive) setDirs((prev) => ({ ...prev, "": { entries: null, loading: false, error: errMsg(e) } })); });
    return () => { alive = false; };
  }, [listDir]);

  const toggle = useCallback((path: string) => {
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
  }, [expanded, dirs, fetchDir]);

  const refresh = useCallback((path: string) => {
    fetchDir(path);
  }, [fetchDir]);

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
          setDirs((prev) => ({ ...prev, [parent]: { entries: r.entries, loading: false, error: null } }));
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

// ---- Tree rows --------------------------------------------------------------

function TreeRow({
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

// ---- File viewer / editor ---------------------------------------------------
// Renders the selected file's content, branching on the backend's returned
// `kind`. Text files (markdown or code) can be switched to an editable
// CodeMirror and saved via writeFile when editing is permitted. Markdown is
// edited as source with a live preview available. When writeFile is omitted the
// viewer is strictly read-only (no Edit button, CodeMirror not editable).

function FileViewer({
  path, readFile, writeFile, onSaved,
}: {
  path: string | null;
  readFile: (path: string) => Promise<FileContent>;
  // When omitted the viewer is read-only.
  writeFile?: (path: string, content: string) => Promise<void>;
  onSaved: (path: string) => void;
}) {
  const { theme } = useTheme();
  const [file, setFile] = useState<FileContent | null>(null);
  // The viewer is remounted per selected path (keyed in the parent), so `file`
  // starts null and `loading` starts true whenever there is a path — no
  // synchronous reset in the effect body is needed.
  const [loading, setLoading] = useState(path !== null);
  const [preview, setPreview] = useState(true);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const canEdit = !!writeFile;

  useEffect(() => {
    if (!path) return;
    let alive = true;
    void readFile(path)
      .then((r) => { if (alive) setFile(r); })
      .catch((e) => { if (alive) toast.error(errMsg(e)); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [readFile, path]);

  const dirty = editing && file != null && draft !== file.content;

  const startEdit = useCallback(() => {
    if (!file) return;
    setDraft(file.content);
    setEditing(true);
    // For markdown the editable CodeMirror only mounts in Source view; entering
    // Edit from Preview would otherwise show Save/Cancel over no editable
    // surface. Force Source so Edit always reveals the editor.
    setPreview(false);
  }, [file]);

  const save = useCallback(() => {
    if (!path || !file || !writeFile) return;
    setSaving(true);
    void writeFile(path, draft)
      .then(() => {
        // Fold the saved text into `file` so the dirty flag clears while the
        // editor stays open on the same content. Leave `size` as-is: draft.length
        // counts UTF-16 code units, which diverges from the server's UTF-8 byte
        // size for multibyte content; the stale byte size is only surfaced on the
        // binary/too_large paths, not for editable text.
        setFile((prev) => (prev ? { ...prev, content: draft } : prev));
        toast.success("Saved");
        onSaved(path);
      })
      .catch((e) => toast.error(errMsg(e)))
      .finally(() => setSaving(false));
  }, [path, file, writeFile, draft, onSaved]);

  const cmTheme = (theme === "dark" || (theme === "system" && typeof window !== "undefined" && window.matchMedia?.("(prefers-color-scheme: dark)").matches))
    ? "dark"
    : "light";

  if (!path) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Select a file to view</div>;
  }
  if (loading) {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" /> Loading…
      </div>
    );
  }
  if (!file) return null;

  if (file.kind === "binary") {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Binary file — not shown ({file.size} bytes)</div>;
  }
  if (file.kind === "too_large") {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">File too large to display ({file.size} bytes)</div>;
  }

  const md = isMarkdown(path);
  const shown = editing ? draft : file.content;

  // Read-only viewers surface no edit controls at all.
  const editControls = !canEdit ? null : editing ? (
    <>
      {dirty && <span data-testid="dirty" title="Unsaved changes" className="text-xs text-amber-600 dark:text-amber-400">●</span>}
      {/* Enabled whenever editing so a re-save always works; the dot flags
          unsaved changes rather than the button's disabled state. */}
      <Button size="sm" onClick={save} disabled={saving}>
        {saving ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
      </Button>
      <Button size="sm" variant="ghost" onClick={() => setEditing(false)} disabled={saving}>Cancel</Button>
    </>
  ) : (
    <Button size="sm" variant="ghost" onClick={startEdit}>
      <Pencil className="size-3.5" /> Edit
    </Button>
  );

  if (md) {
    // In edit mode the Source view becomes an editable CodeMirror; Preview
    // always renders the current (possibly-edited) markdown.
    const showSource = !preview;
    return (
      <div className="flex h-full flex-col">
        <div className="flex shrink-0 items-center gap-1 border-b px-2 py-1">
          <Button size="sm" variant={preview ? "default" : "ghost"} onClick={() => setPreview(true)}>Preview</Button>
          <Button size="sm" variant={preview ? "ghost" : "default"} onClick={() => setPreview(false)}>Source</Button>
          {editControls && <div className="ml-auto flex items-center gap-1">{editControls}</div>}
        </div>
        <div className="min-h-0 flex-1 overflow-auto">
          {!showSource ? (
            <div data-testid="md-preview" className="prose prose-sm dark:prose-invert max-w-none p-4">
              <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
                {shown}
              </ReactMarkdown>
            </div>
          ) : editing ? (
            <CodeMirror
              value={draft}
              theme={cmTheme}
              extensions={langFor(path)}
              onChange={setDraft}
            />
          ) : (
            <pre data-testid="md-source" className="whitespace-pre-wrap p-4 font-mono text-xs">{file.content}</pre>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {editControls && <div className="flex shrink-0 items-center justify-end gap-1 border-b px-2 py-1">{editControls}</div>}
      <div className="min-h-0 flex-1 overflow-auto" data-testid="code-view">
        <CodeMirror
          value={shown}
          theme={cmTheme}
          editable={editing}
          readOnly={!editing}
          extensions={langFor(path)}
          onChange={editing ? setDraft : undefined}
        />
      </div>
    </div>
  );
}

// ---- Create / rename dialogs ------------------------------------------------

// Fully controlled so the field never needs an effect to reset — the parent
// seeds `value` in the handler that opens the dialog.
function PathPromptDialog({
  open, title, label, value, onValueChange, submitLabel, onSubmit, onOpenChange,
}: {
  open: boolean;
  title: string;
  label: string;
  value: string;
  onValueChange: (value: string) => void;
  submitLabel: string;
  onSubmit: (value: string) => void;
  onOpenChange: (open: boolean) => void;
}) {
  const submit = () => {
    const v = value.trim();
    if (!v) return;
    onSubmit(v);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">{label}</span>
          <Input
            autoFocus
            value={value}
            onChange={(e) => onValueChange(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); submit(); } }}
          />
        </label>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={submit} disabled={!value.trim()}>{submitLabel}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---- Browser ----------------------------------------------------------------

type NewKind = "file" | "dir";
interface RenameState { path: string; isDir: boolean }
interface ConfirmState { title: string; body: string; actionLabel: string; run: () => void }

function FileBrowserInner({
  listDir, readFile, writeFile, createFile, renameFile, deleteFile, readOnly,
}: FileBrowserProps) {
  const tree = useFileTree(listDir);
  const [selected, setSelected] = useState<string | null>(null);
  const [newKind, setNewKind] = useState<NewKind | null>(null);
  const [rename, setRename] = useState<RenameState | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  // Shared text for whichever path-prompt dialog is open; seeded by the opener.
  const [promptValue, setPromptValue] = useState("");

  // Each affordance requires both its callback AND write access. readOnly forces
  // everything off regardless of which callbacks were supplied.
  const canEdit = !readOnly && !!writeFile;
  const canCreate = !readOnly && !!createFile;
  const canRename = !readOnly && !!renameFile;
  const canDelete = !readOnly && !!deleteFile;

  const refreshParent = useCallback((path: string) => {
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
        if (tree.dirs[ancestor]?.entries != null) { tree.refresh(ancestor); break; }
        if (ancestor === "") break;
        ancestor = dirName(ancestor);
      }
    }
  }, [tree]);

  const doCreate = useCallback((kind: NewKind, path: string) => {
    if (!createFile) return;
    void createFile(path, kind)
      .then(() => {
        toast.success(`Created ${path}`);
        refreshParent(path);
        setNewKind(null);
      })
      .catch((e) => toast.error(errMsg(e)));
  }, [createFile, refreshParent]);

  const doRename = useCallback((from: string, to: string) => {
    if (!renameFile) return;
    void renameFile(from, to)
      .then(() => {
        toast.success(`Renamed to ${to}`);
        refreshParent(from);
        refreshParent(to);
        if (selected === from) setSelected(to);
        setRename(null);
      })
      .catch((e) => toast.error(errMsg(e)));
  }, [renameFile, refreshParent, selected]);

  const submitRename = useCallback((to: string) => {
    if (!rename) return;
    if (to === rename.path) { setRename(null); return; }
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
        body: state === "exists"
          ? `"${to}" already exists and will be overwritten.`
          : `Could not verify whether "${to}" already exists; renaming may overwrite it.`,
        actionLabel: "Overwrite",
        run: () => doRename(from, to),
      });
    });
  }, [rename, tree, doRename]);

  const doDelete = useCallback((path: string) => {
    if (!deleteFile) return;
    void deleteFile(path)
      .then(() => {
        toast.success(`Deleted ${path}`);
        refreshParent(path);
        if (selected === path) setSelected(null);
      })
      .catch((e) => toast.error(errMsg(e)));
  }, [deleteFile, refreshParent, selected]);

  const onDelete = useCallback((entry: FileEntry, path: string) => {
    setConfirm({
      title: `Delete ${entry.isDir ? "folder" : "file"}?`,
      body: entry.isDir
        ? `"${path}" and everything inside it will be permanently deleted.`
        : `"${path}" will be permanently deleted.`,
      actionLabel: "Delete",
      run: () => doDelete(path),
    });
  }, [doDelete]);

  const onRename = useCallback((entry: FileEntry, path: string) => {
    setPromptValue(path);
    setRename({ path, isDir: entry.isDir });
  }, []);

  const openNew = useCallback((kind: NewKind) => {
    setPromptValue("");
    setNewKind(kind);
  }, []);

  const root = tree.dirs[""];
  let rootBody: ReactNode;
  if (root?.error && !root.entries) {
    rootBody = <div className="p-3 text-xs text-muted-foreground">{root.error}</div>;
  } else if (root?.entries == null) {
    rootBody = (
      <div className="flex items-center gap-2 p-3 text-xs text-muted-foreground">
        <Loader2 className="size-3.5 animate-spin" /> Loading…
      </div>
    );
  } else if (root.entries.length === 0) {
    rootBody = <div className="p-3 text-xs text-muted-foreground">Working directory is empty</div>;
  } else {
    rootBody = (
      <div className="py-1">
        {root.entries.map((e) => (
          <TreeRow
            key={e.name}
            tree={tree}
            path={e.name}
            entry={e}
            depth={0}
            selected={selected}
            onSelect={setSelected}
            onRename={canRename ? onRename : undefined}
            onDelete={canDelete ? onDelete : undefined}
          />
        ))}
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 gap-0 overflow-hidden rounded border">
      <aside className="flex w-64 shrink-0 flex-col overflow-hidden border-r">
        <div className="flex shrink-0 items-center gap-1 border-b px-2 py-1">
          <span className="mr-auto text-xs font-medium text-muted-foreground">Files</span>
          {canCreate && (
            <>
              <Button size="icon-sm" variant="ghost" aria-label="New file" title="New file" onClick={() => openNew("file")}>
                <FilePlus className="size-3.5" />
              </Button>
              <Button size="icon-sm" variant="ghost" aria-label="New folder" title="New folder" onClick={() => openNew("dir")}>
                <FolderPlus className="size-3.5" />
              </Button>
            </>
          )}
        </div>
        <div className="min-h-0 flex-1 overflow-auto">{rootBody}</div>
      </aside>
      <div className="min-w-0 flex-1 overflow-hidden">
        <FileViewer
          key={selected ?? "__none"}
          path={selected}
          readFile={readFile}
          writeFile={canEdit ? writeFile : undefined}
          onSaved={refreshParent}
        />
      </div>

      {canCreate && (
        <PathPromptDialog
          open={newKind !== null}
          title={newKind === "dir" ? "New folder" : "New file"}
          label="Path (relative to working directory)"
          value={promptValue}
          onValueChange={setPromptValue}
          submitLabel="Create"
          onOpenChange={(o) => { if (!o) setNewKind(null); }}
          onSubmit={(v) => newKind && doCreate(newKind, v)}
        />
      )}

      {canRename && (
        <PathPromptDialog
          open={rename !== null}
          title="Rename / move"
          label="New path (relative to working directory)"
          value={promptValue}
          onValueChange={setPromptValue}
          submitLabel="Rename"
          onOpenChange={(o) => { if (!o) setRename(null); }}
          onSubmit={submitRename}
        />
      )}

      <AlertDialog open={confirm !== null} onOpenChange={(o) => { if (!o) setConfirm(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{confirm?.title}</AlertDialogTitle>
            <AlertDialogDescription>{confirm?.body}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => { confirm?.run(); setConfirm(null); }}
            >
              {confirm?.actionLabel}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// Keyed by sourceKey so the whole tree model resets when the source changes.
export default function FileBrowser(props: FileBrowserProps) {
  return <FileBrowserInner key={props.sourceKey} {...props} />;
}
