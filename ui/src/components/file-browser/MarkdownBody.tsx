import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import CodeMirror from "@uiw/react-codemirror";
import { langFor } from "./languages";
import "@/styles/hljs-github.css";

export function MarkdownBody({
  path,
  content,
  preview,
  editing,
  draft,
  setDraft,
  cmTheme,
}: {
  path: string;
  content: string;
  preview: boolean;
  editing: boolean;
  draft: string;
  setDraft: (value: string) => void;
  cmTheme: "dark" | "light";
}) {
  // In edit mode the Source view becomes an editable CodeMirror; Preview
  // always renders the current (possibly-edited) markdown.
  const showSource = !preview;
  const shown = editing ? draft : content;
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      {!showSource ? (
        <div data-testid="md-preview" className="prose prose-sm dark:prose-invert max-w-none p-4">
          <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
            {shown}
          </ReactMarkdown>
        </div>
      ) : editing ? (
        <CodeMirror value={draft} theme={cmTheme} extensions={langFor(path)} onChange={setDraft} />
      ) : (
        <pre data-testid="md-source" className="whitespace-pre-wrap p-4 font-mono text-xs">
          {content}
        </pre>
      )}
    </div>
  );
}
