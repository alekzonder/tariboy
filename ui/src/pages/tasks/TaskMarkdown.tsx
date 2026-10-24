import { lazy, type ComponentProps } from "react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { LazyBoundary } from "@/components/LazyBoundary"
import { markdownStyles, openMarkdownLink } from "./taskMarkdownShared"

// TipTap and ProseMirror load with the first editor, never for a viewer.
const RichMarkdownEditor = lazy(() => import("./TaskMarkdownEditor"))

export function MarkdownContent({ children }: { children: string }) {
  return <div className={markdownStyles}><ReactMarkdown remarkPlugins={[remarkGfm]} components={{
    a: ({ href, children: content, title }) => <a href={href} title={title} onClick={(event) => openMarkdownLink(event, href)}>{content}</a>,
  }}>{children}</ReactMarkdown></div>
}

/** Rich text or the Markdown source behind it. */
export type MarkdownMode = "rich" | "source"

/**
 * The mode switch as the style layer draws it: a segmented control on the
 * section's label row rather than a toolbar strip inside the editor box. It is
 * exported separately so the owner of the label row renders it there; an editor
 * given no explicit mode still shows its own switch and keeps its own state.
 */
export function MarkdownModeSegment({ label, mode, onModeChange, disabled = false, names = ["Rich text", "Markdown"] }: {
  label: string
  mode: MarkdownMode
  onModeChange: (mode: MarkdownMode) => void
  disabled?: boolean
  /** The names of the rich and the source mode. */
  names?: readonly [string, string]
}) {
  return <div role="group" aria-label={label} className="flex shrink-0 gap-0.5 rounded-[9px] bg-muted p-0.5">
    {([["rich", names[0]], ["source", names[1]]] as const).map(([value, text]) => (
      <button key={value} type="button" aria-pressed={mode === value} disabled={disabled}
        onClick={() => onModeChange(value)}
        className={`h-[22px] rounded-[7px] px-[9px] text-[11.5px] disabled:opacity-50 ${mode === value
          ? "bg-card font-medium text-foreground shadow-[var(--raise)]"
          : "text-muted-foreground hover:text-foreground"}`}>
        {text}
      </button>
    ))}
  </div>
}

export function MarkdownEditor(props: ComponentProps<typeof RichMarkdownEditor>) {
  return <LazyBoundary resetKey="markdown-editor"><RichMarkdownEditor {...props} /></LazyBoundary>
}
