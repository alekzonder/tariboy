import { useTheme, type Theme } from "@/components/themeContext";
import type { FileContent } from "@/lib/api";
import { isMarkdown } from "./paths";
import { FileViewerToolbar } from "./FileViewerToolbar";
import { FileViewerStatus } from "./FileViewerStatus";
import { MarkdownBody } from "./MarkdownBody";
import { CodeBody } from "./CodeBody";
import { useFileDraft } from "./useFileDraft";

function codeMirrorTheme(theme: Theme): "dark" | "light" {
  return theme === "dark" ||
    (theme === "system" &&
      typeof window !== "undefined" &&
      window.matchMedia?.("(prefers-color-scheme: dark)").matches)
    ? "dark"
    : "light";
}

export function FileViewer({
  path,
  readFile,
  writeFile,
  onSaved,
}: {
  path: string | null;
  readFile: (path: string) => Promise<FileContent>;
  // When omitted the viewer is read-only.
  writeFile?: (path: string, content: string) => Promise<void>;
  onSaved: (path: string) => void;
}) {
  const { theme } = useTheme();
  const d = useFileDraft(path, readFile, writeFile, onSaved);
  const { file, editing, draft, setDraft } = d;
  const cmTheme = codeMirrorTheme(theme);

  if (!path) return <FileViewerStatus status="empty" />;
  if (d.loading) return <FileViewerStatus status="loading" />;
  if (!file) return null;
  if (file.kind === "binary" || file.kind === "too_large") {
    return <FileViewerStatus status={file.kind} size={file.size} />;
  }

  const md = isMarkdown(path);
  return (
    <div className="flex h-full flex-col">
      <FileViewerToolbar
        markdown={md}
        preview={d.preview}
        setPreview={d.setPreview}
        canEdit={!!writeFile}
        editing={editing}
        dirty={d.dirty}
        saving={d.saving}
        save={d.save}
        startEdit={d.startEdit}
        cancelEdit={d.cancelEdit}
      />
      {md ? (
        <MarkdownBody
          path={path}
          content={file.content}
          preview={d.preview}
          editing={editing}
          draft={draft}
          setDraft={setDraft}
          cmTheme={cmTheme}
        />
      ) : (
        <CodeBody
          path={path}
          value={editing ? draft : file.content}
          editing={editing}
          setDraft={setDraft}
          cmTheme={cmTheme}
        />
      )}
    </div>
  );
}
