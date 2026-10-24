import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import CodeMirror from "@uiw/react-codemirror";
import { Loader2 } from "lucide-react";
import { useTheme } from "@/components/themeContext";
import type { FileContent } from "@/lib/api";
import { errMsg, isMarkdown } from "./paths";
import { langFor } from "./languages";
import { FileViewerToolbar } from "./FileViewerToolbar";
import "@/styles/hljs-github.css";

export function FileViewer({
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

  if (md) {
    // In edit mode the Source view becomes an editable CodeMirror; Preview
    // always renders the current (possibly-edited) markdown.
    const showSource = !preview;
    return (
      <div className="flex h-full flex-col">
        <FileViewerToolbar
          markdown={md} preview={preview} setPreview={setPreview} canEdit={canEdit}
          editing={editing} dirty={dirty} saving={saving} save={save}
          startEdit={startEdit} cancelEdit={() => setEditing(false)}
        />
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
      <FileViewerToolbar
        markdown={md} preview={preview} setPreview={setPreview} canEdit={canEdit}
        editing={editing} dirty={dirty} saving={saving} save={save}
        startEdit={startEdit} cancelEdit={() => setEditing(false)}
      />
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
