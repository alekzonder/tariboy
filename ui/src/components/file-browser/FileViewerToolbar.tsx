import { Loader2, Pencil, Save } from "lucide-react";
import { Button } from "@/components/ui/button";

export function FileViewerToolbar({
  markdown,
  preview,
  setPreview,
  canEdit,
  editing,
  dirty,
  saving,
  save,
  startEdit,
  cancelEdit,
}: {
  markdown: boolean;
  preview: boolean;
  setPreview: (value: boolean) => void;
  canEdit: boolean;
  editing: boolean;
  dirty: boolean;
  saving: boolean;
  save: () => void;
  startEdit: () => void;
  cancelEdit: () => void;
}) {
  // Read-only viewers surface no edit controls at all.
  const editControls = !canEdit ? null : editing ? (
    <>
      {dirty && (
        <span
          data-testid="dirty"
          title="Unsaved changes"
          className="text-xs text-amber-600 dark:text-amber-400"
        >
          ●
        </span>
      )}
      {/* Enabled whenever editing so a re-save always works; the dot flags
          unsaved changes rather than the button's disabled state. */}
      <Button size="sm" onClick={save} disabled={saving}>
        {saving ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}{" "}
        Save
      </Button>
      <Button size="sm" variant="ghost" onClick={cancelEdit} disabled={saving}>
        Cancel
      </Button>
    </>
  ) : (
    <Button size="sm" variant="ghost" onClick={startEdit}>
      <Pencil className="size-3.5" /> Edit
    </Button>
  );

  if (!markdown) {
    return (
      editControls && (
        <div className="flex shrink-0 items-center justify-end gap-1 border-b px-2 py-1">
          {editControls}
        </div>
      )
    );
  }
  return (
    <div className="flex shrink-0 items-center gap-1 border-b px-2 py-1">
      <Button size="sm" variant={preview ? "default" : "ghost"} onClick={() => setPreview(true)}>
        Preview
      </Button>
      <Button size="sm" variant={preview ? "ghost" : "default"} onClick={() => setPreview(false)}>
        Source
      </Button>
      {editControls && <div className="ml-auto flex items-center gap-1">{editControls}</div>}
    </div>
  );
}
