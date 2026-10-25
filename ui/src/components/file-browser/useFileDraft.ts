import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import type { AgentFileContent } from "@/lib/api";
import { errorText } from "@/lib/errors";

// Loads one file and owns its edit draft, save state and Preview/Source choice.
export function useFileDraft(
  path: string | null,
  readFile: (path: string) => Promise<AgentFileContent>,
  writeFile: ((path: string, content: string) => Promise<void>) | undefined,
  onSaved: (path: string) => void,
) {
  const [file, setFile] = useState<AgentFileContent | null>(null);
  // The viewer is remounted per selected path (keyed in the parent), so `file`
  // starts null and `loading` starts true whenever there is a path — no
  // synchronous reset in the effect body is needed.
  const [loading, setLoading] = useState(path !== null);
  const [preview, setPreview] = useState(true);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!path) return;
    let alive = true;
    void readFile(path)
      .then((r) => {
        if (alive) setFile(r);
      })
      .catch((e) => {
        if (alive) toast.error(errorText(e));
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [readFile, path]);

  const dirty = editing && file != null && draft !== (file.content ?? "");

  const startEdit = useCallback(() => {
    if (!file) return;
    setDraft(file.content ?? "");
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
      .catch((e) => toast.error(errorText(e)))
      .finally(() => setSaving(false));
  }, [path, file, writeFile, draft, onSaved]);

  const cancelEdit = useCallback(() => setEditing(false), []);

  return {
    file,
    loading,
    preview,
    setPreview,
    editing,
    draft,
    setDraft,
    dirty,
    saving,
    startEdit,
    save,
    cancelEdit,
  };
}
