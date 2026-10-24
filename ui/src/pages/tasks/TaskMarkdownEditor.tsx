import { useEffect, useState } from "react";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import CodeBlock from "@tiptap/extension-code-block";
import { Markdown } from "@tiptap/markdown";
import { OrderedList, TaskItem, TaskList } from "@tiptap/extension-list";
import { TableKit } from "@tiptap/extension-table";
import remarkGfm from "remark-gfm";
import remarkParse from "remark-parse";
import { unified } from "unified";
import {
  Bold,
  Italic,
  Strikethrough,
  List,
  ListOrdered,
  ListTodo,
  Link,
  Quote,
  Code,
  SquareCode,
  Table,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { MarkdownModeSegment, type MarkdownMode } from "./TaskMarkdown";
import { markdownFence, markdownStyles, openMarkdownLink } from "./taskMarkdownShared";

const extensions = [
  StarterKit.configure({
    codeBlock: false,
    orderedList: false,
    underline: false,
    trailingNode: false,
    link: { openOnClick: false },
  }),
  CodeBlock.extend({
    renderMarkdown(node, helpers) {
      const content = node.content ? helpers.renderChildren(node.content) : "";
      const fence = markdownFence(content);
      return `${fence}${node.attrs?.language ?? ""}\n${content}\n${fence}`;
    },
  }),
  OrderedList.extend({
    parseMarkdown(token, helpers) {
      const parsed = OrderedList.config.parseMarkdown?.call(this, token, helpers);
      if (parsed && !Array.isArray(parsed)) {
        // Tiptap's ordered-list parser omits the required paragraph for empty items.
        for (const item of parsed.content ?? []) {
          if (item.type === "listItem" && !item.content?.length)
            item.content = [{ type: "paragraph" }];
        }
      }
      return parsed ?? [];
    },
  }),
  TaskList,
  TaskItem.configure({ nested: true }),
  TableKit,
  Markdown,
];
const markdownParser = unified().use(remarkParse).use(remarkGfm);
function markdownStructure(value: string) {
  return JSON.stringify(markdownParser.parse(value), (key, field) =>
    key === "position" ? undefined : field,
  );
}

/**
 * Tiptap serializes a trailing empty paragraph as `&nbsp;` so the empty block
 * survives a Markdown round trip. Markdown has no trailing blank paragraph, so
 * that entity is content the writer never typed, and `richMarkdown` cannot
 * round-trip it back, which shows it as a literal code block. Trailing blank
 * space carries no Markdown either way, so drop both from the emitted string
 * and leave the editor document, and the writer's caret, alone. This runs on
 * every keystroke, so it stays a trim of the serialized text rather than a
 * second pass over the document.
 */
function editorMarkdown(editor: Editor) {
  return editor.getMarkdown().replace(/(?:\s|&nbsp;)+$/, "");
}

function richMarkdown(value: string, editor: Editor) {
  return (
    editor.markdown?.instance
      .lexer(value)
      .map((token) => {
        const raw = token.raw ?? "";
        if (!raw.trim()) return raw;
        try {
          const parsed = editor.schema.nodeFromJSON(editor.markdown!.parse(raw));
          parsed.check();
          if (
            markdownStructure(raw) ===
            markdownStructure(editor.markdown!.serialize(parsed.toJSON()))
          )
            return raw;
        } catch {
          // Render any block Tiptap cannot parse safely as editable literal text.
        }
        const content = raw.replace(/\n+$/, "");
        const fence = markdownFence(content);
        return `${fence}\n${content}\n${fence}\n`;
      })
      .join("") ?? value
  );
}

export default function MarkdownEditor({
  value,
  onChange,
  id,
  placeholder,
  disabled = false,
  mode,
  surface = "muted",
}: {
  value: string;
  onChange: (value: string) => void;
  id?: string;
  placeholder?: string;
  disabled?: boolean;
  /** Lift the mode out of the editor: when set, the caller owns the switch
   *  and renders MarkdownModeSegment on the section's label row. */
  mode?: MarkdownMode;
  /** Which fill the editor sits on. `card` is for an editor nested inside a
   *  `--muted` block, where the same tone twice would read as one surface. */
  surface?: "muted" | "card";
}) {
  const [source, setSource] = useState(false);
  const [showLink, setShowLink] = useState(false);
  const [linkURL, setLinkURL] = useState("");
  const [linkError, setLinkError] = useState("");
  const editor = useEditor({
    extensions,
    content: "",
    editable: !disabled,
    shouldRerenderOnTransaction: true,
    editorProps: {
      attributes: {
        ...(id ? { id } : {}),
        role: "textbox",
        "aria-multiline": "true",
        ...(placeholder ? { "aria-label": placeholder, "data-placeholder": placeholder } : {}),
        class: `${markdownStyles} min-h-32 px-3 py-2.75 outline-none focus-visible:ring-2 focus-visible:ring-ring`,
      },
    },
    onUpdate: ({ editor: current }) => onChange(editorMarkdown(current)),
  });
  const showSource = mode ? mode === "source" : source;

  useEffect(() => {
    if (!editor) return;
    editor.setEditable(!disabled && !showSource, false);
    if (!showSource) {
      const content = richMarkdown(value, editor);
      if (editorMarkdown(editor) === content) return;
      editor.commands.setContent(content, { contentType: "markdown", emitUpdate: false });
    }
  }, [editor, value, disabled, showSource]);

  const actions = editor
    ? [
        {
          label: "Bold",
          icon: Bold,
          active: editor.isActive("bold"),
          run: () => editor.chain().focus().toggleBold().run(),
        },
        {
          label: "Italic",
          icon: Italic,
          active: editor.isActive("italic"),
          run: () => editor.chain().focus().toggleItalic().run(),
        },
        {
          label: "Strikethrough",
          icon: Strikethrough,
          active: editor.isActive("strike"),
          run: () => editor.chain().focus().toggleStrike().run(),
        },
        {
          label: "Bullet list",
          icon: List,
          active: editor.isActive("bulletList"),
          run: () => editor.chain().focus().toggleBulletList().run(),
        },
        {
          label: "Numbered list",
          icon: ListOrdered,
          active: editor.isActive("orderedList"),
          run: () => editor.chain().focus().toggleOrderedList().run(),
        },
        {
          label: "Task list",
          icon: ListTodo,
          active: editor.isActive("taskList"),
          run: () => editor.chain().focus().toggleTaskList().run(),
        },
        {
          label: "Quote",
          icon: Quote,
          active: editor.isActive("blockquote"),
          run: () => editor.chain().focus().toggleBlockquote().run(),
        },
        {
          label: "Inline code",
          icon: Code,
          active: editor.isActive("code"),
          run: () => editor.chain().focus().toggleCode().run(),
        },
        {
          label: "Code block",
          icon: SquareCode,
          active: editor.isActive("codeBlock"),
          run: () => editor.chain().focus().toggleCodeBlock().run(),
        },
        {
          label: "Insert table",
          icon: Table,
          active: false,
          run: () =>
            editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
        },
      ]
    : [];

  function applyLink() {
    if (!editor || disabled) return;
    let url: URL;
    try {
      url = new URL(linkURL);
      if (!["http:", "https:"].includes(url.protocol)) throw new Error("protocol");
    } catch {
      setLinkError("Enter a complete http:// or https:// URL.");
      return;
    }
    if (editor.state.selection.empty && !editor.isActive("link")) {
      editor
        .chain()
        .focus()
        .insertContent({
          type: "text",
          text: linkURL,
          marks: [{ type: "link", attrs: { href: linkURL } }],
        })
        .run();
    } else {
      editor.chain().focus().extendMarkRange("link").setLink({ href: linkURL }).run();
    }
    setShowLink(false);
    setLinkError("");
  }

  /* A field in this style layer is a fill, not a box: the editor is one
     `--muted` block at radius 8, and its chrome rows are separated by
     padding rather than by rules. */
  return (
    <div className={`min-w-0 rounded-md ${surface === "card" ? "bg-card" : "bg-muted"}`}>
      {!mode && (
        <div
          className="flex flex-wrap items-center gap-1 p-1.5 pb-0"
          aria-label="Markdown editing mode"
        >
          <MarkdownModeSegment
            label="Markdown editing mode"
            disabled={disabled}
            mode={showSource ? "source" : "rich"}
            onModeChange={(next) => {
              setSource(next === "source");
              if (next === "source") setShowLink(false);
            }}
          />
        </div>
      )}
      {showSource ? (
        <>
          <Textarea
            id={id}
            aria-label={placeholder}
            placeholder={placeholder}
            disabled={disabled}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            className="min-h-32 rounded-none border-0 bg-transparent font-mono text-body-sm shadow-none focus-visible:ring-0"
          />
        </>
      ) : (
        <>
          <div
            className="flex flex-wrap items-center gap-0.5 p-1.5 pb-0"
            role="group"
            aria-label="Markdown formatting"
          >
            {([1, 2, 3] as const).map((level) => (
              <Button
                key={level}
                type="button"
                size="icon-xs"
                variant={editor?.isActive("heading", { level }) ? "secondary" : "ghost"}
                title={`Heading ${level}`}
                aria-label={`Heading ${level}`}
                aria-pressed={editor?.isActive("heading", { level })}
                disabled={disabled}
                onClick={() => editor?.chain().focus().toggleHeading({ level }).run()}
              >
                H{level}
              </Button>
            ))}
            {actions.map(({ label, icon: Icon, active, run }) => (
              <Button
                key={label}
                type="button"
                size="icon-xs"
                variant={active ? "secondary" : "ghost"}
                title={label}
                aria-label={label}
                aria-pressed={active}
                disabled={disabled}
                onClick={run}
              >
                <Icon />
              </Button>
            ))}
            <Button
              type="button"
              size="icon-xs"
              variant={editor?.isActive("link") ? "secondary" : "ghost"}
              title="Link"
              aria-label="Link"
              aria-pressed={editor?.isActive("link")}
              disabled={disabled}
              onClick={() => {
                setLinkURL(String(editor?.getAttributes("link").href ?? ""));
                setLinkError("");
                setShowLink(!showLink);
              }}
            >
              <Link />
            </Button>
            {editor?.isActive("table") && (
              <>
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => editor.chain().focus().addRowAfter().run()}
                >
                  Add row
                </Button>
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => editor.chain().focus().addColumnAfter().run()}
                >
                  Add column
                </Button>
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => editor.chain().focus().deleteRow().run()}
                >
                  Delete row
                </Button>
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => editor.chain().focus().deleteColumn().run()}
                >
                  Delete column
                </Button>
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => editor.chain().focus().deleteTable().run()}
                >
                  Delete table
                </Button>
              </>
            )}
          </div>
          {showLink && (
            <div className="space-y-1 px-1.5 pt-1.5">
              <div className="flex flex-wrap gap-1">
                <Input
                  aria-label="Link URL"
                  value={linkURL}
                  disabled={disabled}
                  onChange={(event) => setLinkURL(event.target.value)}
                  placeholder="https://example.com"
                  className="min-w-40 flex-1"
                />
                <Button type="button" size="sm" disabled={disabled} onClick={applyLink}>
                  Apply link
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => {
                    editor?.chain().focus().extendMarkRange("link").unsetLink().run();
                    setShowLink(false);
                  }}
                >
                  Remove link
                </Button>
                <Button type="button" size="sm" variant="ghost" onClick={() => setShowLink(false)}>
                  Cancel
                </Button>
              </div>
              {linkError && (
                <p role="alert" className="text-xs text-destructive">
                  {linkError}
                </p>
              )}
            </div>
          )}
          <div className="relative">
            <EditorContent
              editor={editor}
              onClick={(event) =>
                openMarkdownLink(
                  event,
                  (event.target as Element).closest("a")?.getAttribute("href") ?? undefined,
                )
              }
            />
            {editor?.isEmpty && placeholder && (
              <p
                aria-hidden="true"
                className="pointer-events-none absolute top-2.75 left-3 text-body-sm text-muted-foreground"
              >
                {placeholder}
              </p>
            )}
          </div>
        </>
      )}
    </div>
  );
}
