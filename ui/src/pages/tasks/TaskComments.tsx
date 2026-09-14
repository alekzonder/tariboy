import { useEffect, useState } from "react"
import { ChevronDown, Copy } from "lucide-react"
import { toast } from "sonner"
import type { TaskComment, TaskPrincipals, TaskWait } from "@/lib/tasks"
import { MarkdownEditor, MarkdownContent, MarkdownModeSegment, type MarkdownMode } from "./TaskMarkdown"
import { Button } from "@/components/ui/button"
import { SendFilesButton } from "@/components/SendFilesButton"
import { cn } from "@/lib/utils"
import { formatTaskTime } from "./taskTime"
import type { ApiTarget } from "@/lib/api"

/** Shared with the panel: the label, the field fill and the quiet action. */
const LABEL = "text-[11.5px] font-medium tracking-[.02em] text-muted-foreground"
const COUNT = "font-mono text-[11px] tabular-nums text-muted-foreground opacity-75"
const QUIET_ACTION = "h-6 rounded-[7px] px-2.5 text-[12px] font-normal text-muted-foreground hover:bg-accent hover:text-foreground"

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `comment-${Date.now()}-${Math.random()}`
}

/** Two letters is enough to tell one author from another down a column. */
function initials(author: string): string {
  const name = author.replace(/^[a-z]+:/, "")
  return (name.slice(0, 2) || "··").toUpperCase()
}

export default function TaskComments({
  comments,
  waits,
  principals,
  assignee,
  formFirst,
  order,
  onOrderChange,
  onComment,
  onDirtyChange,
  target,
  onUploadingChange,
}: {
  comments: TaskComment[]
  waits: TaskWait[]
  principals: TaskPrincipals | null
  assignee: string
  formFirst: boolean
  order: "newest" | "oldest"
  onOrderChange: (order: "newest" | "oldest") => void
  onDirtyChange: (dirty: boolean) => void
  onComment: (body: string, idempotencyKey: string) => Promise<void>
  target?: ApiTarget
  onUploadingChange: (uploading: boolean) => void
}) {
  const defaultAsk = assignee.replace(/^agent:/, "")
  const [body, setBody] = useState("")
  const [mode, setMode] = useState<MarkdownMode>("rich")
  const [ask, setAsk] = useState({ assignee, principal: defaultAsk })
  const [busy, setBusy] = useState(false)
  const [uploading, setUploading] = useState(false)
  if (ask.assignee !== assignee) setAsk({ assignee, principal: defaultAsk })
  useEffect(() => { onUploadingChange(uploading) }, [uploading, onUploadingChange])
  useEffect(() => { onDirtyChange(Boolean(body.trim()) || busy || uploading) }, [body, busy, uploading, onDirtyChange])
  const openWaits = waits.filter((wait) => !wait.resolved_at)
  // The wait names the comment that asked, so the chip sits on that comment
  // rather than floating above the whole thread.
  const asking = new Set(openWaits.map((wait) => wait.requesting_comment_id))
  const choices = principals
    ? [principals.customer, ...principals.agents].filter(Boolean)
    : []
  if (defaultAsk && !choices.includes(defaultAsk)) choices.push(defaultAsk)

  const send = async (text: string, resetForm: boolean) => {
    if (busy || uploading) return
    setBusy(true)
    try {
      await onComment(text, idempotencyKey())
      if (resetForm) {
        setBody("")
        setAsk({ assignee, principal: defaultAsk })
      }
    } catch {
      // The workspace reports the failure; retain the comment for retry.
    } finally {
      setBusy(false)
    }
  }

  const askText = (text: string) => {
    const principal = ask.principal && !ask.principal.includes(":") ? `agent:${ask.principal}` : ask.principal
    return principal ? `@${principal}\n\n${text}` : text
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!body.trim()) return
    await send(askText(body.trim()), true)
  }

  const copyComment = async (body: string) => {
    try {
      await navigator.clipboard.writeText(body)
      toast.success("Comment Markdown copied")
    } catch (error) {
      toast.error(`Could not copy comment: ${String(error)}`)
    }
  }

  const commentList = (
    /* The grid column is what keeps long words and wide tables inside the
       panel; the vertical rhythm is padding, not a rule between comments. */
    <div key="list" className="task-comment-list">
      {comments.map((comment) => (
        <article key={comment.id} className="flex min-w-0 gap-[9px] py-[7px]">
          <span aria-hidden="true" className="mt-px grid size-[22px] shrink-0 place-items-center rounded-full bg-muted font-mono text-[10.5px] text-muted-foreground">
            {initials(comment.author)}
          </span>
          <div className="flex min-w-0 flex-1 flex-col gap-[3px]">
            <header className="flex min-w-0 items-center gap-2">
              <strong className="shrink-0 font-mono text-[11.5px] font-medium">{comment.author}</strong>
              {asking.has(comment.id) && <span className="inline-flex h-[17px] shrink-0 items-center rounded-[5px] bg-[color-mix(in_oklab,var(--primary)_12%,transparent)] px-1.5 text-[10.5px] font-medium text-primary">waiting for answer</span>}
              <span className="task-comment-actions ml-auto flex shrink-0 items-center gap-1">
                <time className="font-mono text-[11px] tabular-nums text-muted-foreground" dateTime={comment.created_at}
                  title={new Date(comment.created_at).toLocaleString()}>{formatTaskTime(comment.created_at)}</time>
                <Button type="button" size="icon-xs" variant="ghost" className="task-comment-copy size-5 rounded-[6px] text-muted-foreground" aria-label="Copy comment Markdown" title="Copy comment Markdown" onClick={() => void copyComment(comment.body)}><Copy /></Button>
              </span>
            </header>
            <MarkdownContent>{comment.body}</MarkdownContent>
          </div>
        </article>
      ))}
      {comments.length === 0 && <p className="pt-0.5 text-[12px] text-muted-foreground opacity-80">No comments yet.</p>}
    </div>
  )
  const commentForm = (
    <form key="form" className="rounded-[8px] bg-muted p-3" onSubmit={(event) => void submit(event)}>
      <label className="flex min-w-0 items-center gap-2">
        <span className={LABEL}>Ask</span>
        <span className="relative inline-flex min-w-0">
          <select aria-label="Ask" value={ask.principal} onChange={(event) => setAsk({ assignee, principal: event.target.value })}
            className="h-6 w-full min-w-0 appearance-none rounded-[8px] border-0 bg-card pr-7 pl-2.5 text-[11.5px] outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <option value="">No explicit answer needed</option>
            {choices.map((principal) => <option key={principal} value={principal}>{principal}</option>)}
          </select>
          <ChevronDown aria-hidden="true" className="pointer-events-none absolute top-1/2 right-2.5 size-2.5 -translate-y-1/2 opacity-45" />
        </span>
      </label>
      <div>
        <div className="flex items-center gap-2 pb-[7px]">
          <label htmlFor="task-comment" className={LABEL}>Comment</label>
          <MarkdownModeSegment label="Comment editing mode" mode={mode} onModeChange={setMode} disabled={busy} />
          <SendFilesButton daemon={target} disabled={busy} onUploadingChange={setUploading}
            className={cn(QUIET_ACTION, "ml-auto border-0 bg-transparent")}
            onUploaded={(paths) => setBody((current) => current + (current && !current.endsWith("\n") ? "\n" : "") + paths.join("\n"))} />
        </div>
        <MarkdownEditor id="task-comment" placeholder="Comment" value={body} onChange={setBody} disabled={busy} mode={mode} surface="card" />
      </div>
      <div className="flex justify-end gap-1.5">
        {/* Secondary, not ghost: it posts a comment, so it stays a button. */}
        {!body.trim() && <Button type="button" size="sm" variant="secondary" className="h-7 rounded-[7px]" disabled={busy || uploading} onClick={() => void send(askText("Ok"), false)}>Send Ok</Button>}
        <Button type="submit" size="sm" disabled={busy || uploading || !body.trim()}>Send comment</Button>
      </div>
    </form>
  )

  return (
    <section className="task-comments flex min-w-0 flex-col gap-1.5">
      <div className="flex items-center gap-1.5">
        <span className={LABEL}>Comments</span>
        <span className={COUNT}>{comments.length}</span>
        <label className="relative ml-auto inline-flex items-center">
          <span className="sr-only">Comment order</span>
          <select aria-label="Comment order" value={order} onChange={(event) => onOrderChange(event.target.value as "newest" | "oldest")}
            className="h-6 appearance-none rounded-[8px] border-0 bg-muted pr-7 pl-2.5 text-[11.5px] text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <option value="newest">Newest first</option>
            <option value="oldest">Oldest first</option>
          </select>
          <ChevronDown aria-hidden="true" className="pointer-events-none absolute top-1/2 right-2.5 size-2.5 -translate-y-1/2 opacity-45" />
        </label>
      </div>
      {formFirst ? [commentForm, commentList] : [commentList, commentForm]}
    </section>
  )
}
