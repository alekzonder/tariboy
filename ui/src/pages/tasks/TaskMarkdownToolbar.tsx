import type { Editor } from "@tiptap/react";
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
import { parseLinkURL } from "@/lib/markdownLink";

function markdownToolbarActions(editor: Editor) {
  return [
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
  ];
}

function TableActions({ editor, disabled }: { editor: Editor; disabled: boolean }) {
  return (
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
  );
}

/**
 * The rich editor's formatting row and its link form. The link state belongs
 * to the editor, so a draft link survives a trip through source mode.
 */
export default function MarkdownToolbar({
  editor,
  disabled,
  showLink,
  setShowLink,
  linkURL,
  setLinkURL,
  linkError,
  setLinkError,
}: {
  editor: Editor | null;
  disabled: boolean;
  showLink: boolean;
  setShowLink: (show: boolean) => void;
  linkURL: string;
  setLinkURL: (url: string) => void;
  linkError: string;
  setLinkError: (error: string) => void;
}) {
  const actions = editor ? markdownToolbarActions(editor) : [];

  function applyLink() {
    if (!editor || disabled) return;
    if (!parseLinkURL(linkURL)) {
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

  return (
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
        {editor?.isActive("table") && <TableActions editor={editor} disabled={disabled} />}
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
    </>
  );
}
