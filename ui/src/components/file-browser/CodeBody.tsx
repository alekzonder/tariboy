import CodeMirror from "@uiw/react-codemirror";
import { langFor } from "./languages";

export function CodeBody({
  path,
  value,
  editing,
  setDraft,
  cmTheme,
}: {
  path: string;
  value: string;
  editing: boolean;
  setDraft: (value: string) => void;
  cmTheme: "dark" | "light";
}) {
  return (
    <div className="min-h-0 flex-1 overflow-auto" data-testid="code-view">
      <CodeMirror
        value={value}
        theme={cmTheme}
        editable={editing}
        readOnly={!editing}
        extensions={langFor(path)}
        onChange={editing ? setDraft : undefined}
      />
    </div>
  );
}
