import { lazy, type ComponentProps } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { LazyBoundary } from "@/components/LazyBoundary";
import { markdownStyles, openMarkdownLink, roleMarkdownStyles } from "./taskMarkdownShared";

// TipTap and ProseMirror load with the first editor, never for a viewer.
const RichMarkdownEditor = lazy(() => import("./TaskMarkdownEditor"));

const link: Components["a"] = ({ href, children: content, title }) => (
  <a href={href} title={title} onClick={(event) => openMarkdownLink(event, href)}>
    {content}
  </a>
);

// A ROLE.md loads no images: the reader sees the alt text.
const roleComponents: Components = {
  a: link,
  img: ({ alt }) => <span className="text-muted-foreground italic">{alt}</span>,
};

/** Raw HTML in the Markdown is never rendered: rehype-raw is not loaded. The
 *  `role` variant draws a ROLE.md: smaller headings and no images. */
export function MarkdownContent({ children, variant }: { children: string; variant?: "role" }) {
  return (
    <div className={variant === "role" ? roleMarkdownStyles : markdownStyles}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={variant === "role" ? roleComponents : { a: link }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}

/** Rich text or the Markdown source behind it. */
export type MarkdownMode = "rich" | "source";

/**
 * The mode switch as the style layer draws it: a segmented control on the
 * section's label row rather than a toolbar strip inside the editor box. It is
 * exported separately so the owner of the label row renders it there; an editor
 * given no explicit mode still shows its own switch and keeps its own state.
 */
export function MarkdownModeSegment({
  label,
  mode,
  onModeChange,
  disabled = false,
  names = ["Rich text", "Markdown"],
}: {
  label: string;
  mode: MarkdownMode;
  onModeChange: (mode: MarkdownMode) => void;
  disabled?: boolean;
  /** The names of the rich and the source mode. */
  names?: readonly [string, string];
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="flex shrink-0 gap-0.5 rounded-panel bg-muted p-0.5"
    >
      {(
        [
          ["rich", names[0]],
          ["source", names[1]],
        ] as const
      ).map(([value, text]) => (
        <button
          key={value}
          type="button"
          aria-pressed={mode === value}
          disabled={disabled}
          onClick={() => onModeChange(value)}
          className={`h-5.5 rounded-control px-2.25 text-meta disabled:opacity-50 ${
            mode === value
              ? "bg-card font-medium text-foreground shadow-[var(--raise)]"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {text}
        </button>
      ))}
    </div>
  );
}

export function MarkdownEditor(props: ComponentProps<typeof RichMarkdownEditor>) {
  return (
    <LazyBoundary resetKey="markdown-editor">
      <RichMarkdownEditor {...props} />
    </LazyBoundary>
  );
}
