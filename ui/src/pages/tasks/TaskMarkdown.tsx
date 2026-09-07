import { useEffect, useMemo, useState } from "react"
import { EditorContent, useEditor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { Markdown } from "@tiptap/markdown"
import { TaskItem, TaskList } from "@tiptap/extension-list"
import { TableKit } from "@tiptap/extension-table"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import remarkParse from "remark-parse"
import { unified } from "unified"
import { Bold, Italic, Strikethrough, List, ListOrdered, ListTodo, Link, Quote, Code, SquareCode, Table } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"

const extensions = [
  StarterKit.configure({ underline: false, trailingNode: false, link: { openOnClick: false } }),
  TaskList,
  TaskItem.configure({ nested: true }),
  TableKit,
  Markdown,
]
const markdownParser = unified().use(remarkParse).use(remarkGfm)
const markdownStyles = [
  "task-markdown max-w-none text-sm leading-relaxed [&_p]:my-2 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0",
  "[&_:is(h1,h2,h3,h4,h5,h6)]:mt-4 [&_:is(h1,h2,h3,h4,h5,h6)]:mb-2 [&_:is(h1,h2,h3,h4,h5,h6)]:font-semibold [&_h1]:text-2xl [&_h2]:text-xl [&_h3]:text-lg",
  "[&_strong]:font-bold [&_em]:italic [&_del]:line-through [&_s]:line-through [&_a]:text-primary [&_a]:underline",
  "[&_ul]:my-2 [&_ul]:list-disc [&_ul]:pl-6 [&_ol]:my-2 [&_ol]:list-decimal [&_ol]:pl-6 [&_li]:my-1",
  "[&_blockquote]:my-3 [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-4 [&_blockquote]:text-muted-foreground",
  "[&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_pre]:my-3 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-muted [&_pre]:p-3 [&_pre_code]:p-0 [&_hr]:my-4",
  "[&_table]:my-3 [&_table]:block [&_table]:w-full [&_table]:overflow-x-auto [&_table]:border-collapse [&_td]:min-w-24 [&_td]:border [&_td]:p-2 [&_th]:min-w-24 [&_th]:border [&_th]:bg-muted [&_th]:p-2",
  "[&_ul[data-type=taskList]]:list-none [&_ul[data-type=taskList]]:pl-0 [&_li[data-type=taskItem]]:flex [&_li[data-type=taskItem]]:items-start [&_li[data-type=taskItem]]:gap-2 [&_li[data-type=taskItem]>div]:min-w-0 [&_li[data-type=taskItem]>div]:flex-1 [&_li[data-type=taskItem]>label]:pt-0.5 [&_.task-list-item]:list-none [&_.task-list-item>input]:mr-2",
].join(" ")

function markdownStructure(value: string) {
  return JSON.stringify(markdownParser.parse(value), (key, field) => key === "position" ? undefined : field)
}

export function MarkdownContent({ children }: { children: string }) {
  return <div className={markdownStyles}><ReactMarkdown remarkPlugins={[remarkGfm]}>{children}</ReactMarkdown></div>
}

export function MarkdownEditor({ value, onChange, id, placeholder, disabled = false }: {
  value: string
  onChange: (value: string) => void
  id?: string
  placeholder?: string
  disabled?: boolean
}) {
  const [source, setSource] = useState(false)
  const [showLink, setShowLink] = useState(false)
  const [linkURL, setLinkURL] = useState("")
  const [linkError, setLinkError] = useState("")
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
        class: `${markdownStyles} min-h-32 p-3 outline-none focus-visible:ring-2 focus-visible:ring-ring`,
      },
    },
    onUpdate: ({ editor: current }) => onChange(current.getMarkdown()),
  })
  const richSupported = useMemo(() => {
    if (!editor?.markdown) return false
    if (!value.trim()) return true
    try {
      const parsed = editor.schema.nodeFromJSON(editor.markdown.parse(value))
      parsed.check()
      return markdownStructure(value) === markdownStructure(editor.markdown.serialize(parsed.toJSON()))
    } catch {
      return false
    }
  }, [editor, value])
  const showSource = source || !richSupported

  useEffect(() => {
    if (!editor) return
    editor.setEditable(!disabled && !showSource, false)
    if (!showSource && editor.getMarkdown() !== value) {
      editor.commands.setContent(value, { contentType: "markdown", emitUpdate: false })
    }
  }, [editor, value, disabled, showSource])

  const actions = editor ? [
    { label: "Bold", icon: Bold, active: editor.isActive("bold"), run: () => editor.chain().focus().toggleBold().run() },
    { label: "Italic", icon: Italic, active: editor.isActive("italic"), run: () => editor.chain().focus().toggleItalic().run() },
    { label: "Strikethrough", icon: Strikethrough, active: editor.isActive("strike"), run: () => editor.chain().focus().toggleStrike().run() },
    { label: "Bullet list", icon: List, active: editor.isActive("bulletList"), run: () => editor.chain().focus().toggleBulletList().run() },
    { label: "Numbered list", icon: ListOrdered, active: editor.isActive("orderedList"), run: () => editor.chain().focus().toggleOrderedList().run() },
    { label: "Task list", icon: ListTodo, active: editor.isActive("taskList"), run: () => editor.chain().focus().toggleTaskList().run() },
    { label: "Quote", icon: Quote, active: editor.isActive("blockquote"), run: () => editor.chain().focus().toggleBlockquote().run() },
    { label: "Inline code", icon: Code, active: editor.isActive("code"), run: () => editor.chain().focus().toggleCode().run() },
    { label: "Code block", icon: SquareCode, active: editor.isActive("codeBlock"), run: () => editor.chain().focus().toggleCodeBlock().run() },
    { label: "Insert table", icon: Table, active: false, run: () => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() },
  ] : []

  function applyLink() {
    if (!editor || disabled) return
    let url: URL
    try {
      url = new URL(linkURL)
      if (!["http:", "https:"].includes(url.protocol)) throw new Error("protocol")
    } catch {
      setLinkError("Enter a complete http:// or https:// URL.")
      return
    }
    if (editor.state.selection.empty && !editor.isActive("link")) {
      editor.chain().focus().insertContent({ type: "text", text: linkURL, marks: [{ type: "link", attrs: { href: linkURL } }] }).run()
    } else {
      editor.chain().focus().extendMarkRange("link").setLink({ href: linkURL }).run()
    }
    setShowLink(false)
    setLinkError("")
  }

  return <div className="min-w-0 rounded-lg border border-input bg-background">
    <div className="flex flex-wrap items-center gap-1 border-b p-1" aria-label="Markdown editing mode">
      <Button type="button" size="xs" variant={!showSource ? "secondary" : "ghost"} aria-pressed={!showSource} disabled={disabled || !richSupported} onClick={() => setSource(false)}>Rich text</Button>
      <Button type="button" size="xs" variant={showSource ? "secondary" : "ghost"} aria-pressed={showSource} disabled={disabled} onClick={() => { setSource(true); setShowLink(false) }}>Source Markdown</Button>
    </div>
    {showSource ? <>
      {!richSupported && <p role="status" className="px-3 pt-2 text-xs text-muted-foreground">Source mode preserves Markdown that rich text cannot represent.</p>}
      <Textarea id={id} aria-label={placeholder} placeholder={placeholder} disabled={disabled} value={value} onChange={(event) => onChange(event.target.value)} className="min-h-32 rounded-none border-0 font-mono" />
    </> : <>
      <div className="flex flex-wrap items-center gap-0.5 border-b p-1" role="group" aria-label="Markdown formatting">
        {([1, 2, 3] as const).map((level) => <Button key={level} type="button" size="icon-xs" variant={editor?.isActive("heading", { level }) ? "secondary" : "ghost"} title={`Heading ${level}`} aria-label={`Heading ${level}`} aria-pressed={editor?.isActive("heading", { level })} disabled={disabled} onClick={() => editor?.chain().focus().toggleHeading({ level }).run()}>H{level}</Button>)}
        {actions.map(({ label, icon: Icon, active, run }) => <Button key={label} type="button" size="icon-xs" variant={active ? "secondary" : "ghost"} title={label} aria-label={label} aria-pressed={active} disabled={disabled} onClick={run}><Icon /></Button>)}
        <Button type="button" size="icon-xs" variant={editor?.isActive("link") ? "secondary" : "ghost"} title="Link" aria-label="Link" aria-pressed={editor?.isActive("link")} disabled={disabled} onClick={() => { setLinkURL(String(editor?.getAttributes("link").href ?? "")); setLinkError(""); setShowLink(!showLink) }}><Link /></Button>
        {editor?.isActive("table") && <>
          <Button type="button" size="xs" variant="ghost" disabled={disabled} onClick={() => editor.chain().focus().addRowAfter().run()}>Add row</Button>
          <Button type="button" size="xs" variant="ghost" disabled={disabled} onClick={() => editor.chain().focus().addColumnAfter().run()}>Add column</Button>
          <Button type="button" size="xs" variant="ghost" disabled={disabled} onClick={() => editor.chain().focus().deleteRow().run()}>Delete row</Button>
          <Button type="button" size="xs" variant="ghost" disabled={disabled} onClick={() => editor.chain().focus().deleteColumn().run()}>Delete column</Button>
          <Button type="button" size="xs" variant="ghost" disabled={disabled} onClick={() => editor.chain().focus().deleteTable().run()}>Delete table</Button>
        </>}
      </div>
      {showLink && <div className="space-y-1 border-b p-2">
        <div className="flex flex-wrap gap-1">
          <Input aria-label="Link URL" value={linkURL} disabled={disabled} onChange={(event) => setLinkURL(event.target.value)} placeholder="https://example.com" className="min-w-40 flex-1" />
          <Button type="button" size="sm" disabled={disabled} onClick={applyLink}>Apply link</Button>
          <Button type="button" size="sm" variant="ghost" disabled={disabled} onClick={() => { editor?.chain().focus().extendMarkRange("link").unsetLink().run(); setShowLink(false) }}>Remove link</Button>
          <Button type="button" size="sm" variant="ghost" onClick={() => setShowLink(false)}>Cancel</Button>
        </div>
        {linkError && <p role="alert" className="text-xs text-destructive">{linkError}</p>}
      </div>}
      <div className="relative">
        <EditorContent editor={editor} />
        {editor?.isEmpty && placeholder && <p aria-hidden="true" className="pointer-events-none absolute top-3 left-3 text-sm text-muted-foreground">{placeholder}</p>}
      </div>
    </>}
  </div>
}
