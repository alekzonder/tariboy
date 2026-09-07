import { useEffect, useState } from "react"
import type { TaskComment, TaskPrincipals, TaskWait } from "@/lib/tasks"
import { MarkdownEditor, MarkdownContent } from "./TaskMarkdown"
import { Button } from "@/components/ui/button"
import { SendFilesButton } from "@/components/SendFilesButton"
import type { ApiTarget } from "@/lib/api"

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `comment-${Date.now()}-${Math.random()}`
}

export default function TaskComments({
  comments,
  waits,
  principals,
  assignee,
  formFirst,
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
  onDirtyChange: (dirty: boolean) => void
  onComment: (body: string, idempotencyKey: string) => Promise<void>
  target?: ApiTarget
  onUploadingChange: (uploading: boolean) => void
}) {
  const defaultAsk = assignee.replace(/^agent:/, "")
  const [body, setBody] = useState("")
  const [ask, setAsk] = useState({ assignee, principal: defaultAsk })
  const [busy, setBusy] = useState(false)
  const [uploading, setUploading] = useState(false)
  if (ask.assignee !== assignee) setAsk({ assignee, principal: defaultAsk })
  useEffect(() => { onUploadingChange(uploading) }, [uploading, onUploadingChange])
  useEffect(() => { onDirtyChange(Boolean(body.trim()) || busy || uploading) }, [body, busy, uploading, onDirtyChange])
  const openWaits = waits.filter((wait) => !wait.resolved_at)
  const choices = principals
    ? [principals.customer, ...principals.agents].filter(Boolean)
    : []
  if (defaultAsk && !choices.includes(defaultAsk)) choices.push(defaultAsk)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (busy || uploading || !body.trim()) return
    setBusy(true)
    try {
      const principal = ask.principal && !ask.principal.includes(":") ? `agent:${ask.principal}` : ask.principal
      const text = principal ? `@${principal}\n\n${body.trim()}` : body.trim()
      await onComment(text, idempotencyKey())
      setBody("")
      setAsk({ assignee, principal: defaultAsk })
    } catch {
      // The workspace reports the failure; retain the comment for retry.
    } finally {
      setBusy(false)
    }
  }

  const commentList = (
    <div key="list" className="task-comment-list">
      {comments.map((comment) => (
        <article key={comment.id}>
          <header><strong>{comment.author}</strong><time>{new Date(comment.created_at).toLocaleString()}</time></header>
          <MarkdownContent>{comment.body}</MarkdownContent>
        </article>
      ))}
    </div>
  )
  const commentForm = (
    <form key="form" onSubmit={(event) => void submit(event)}>
      <label>
        Ask
        <select aria-label="Ask" value={ask.principal} onChange={(event) => setAsk({ assignee, principal: event.target.value })}>
          <option value="">No explicit answer needed</option>
          {choices.map((principal) => <option key={principal} value={principal}>{principal}</option>)}
        </select>
      </label>
      <div><div className="flex items-center justify-between gap-2"><label htmlFor="task-comment">Comment</label>
        <SendFilesButton daemon={target} disabled={busy} onUploadingChange={setUploading}
          onUploaded={(paths) => setBody((current) => current + (current && !current.endsWith("\n") ? "\n" : "") + paths.join("\n"))} />
        </div>
        <MarkdownEditor id="task-comment" placeholder="Comment" value={body} onChange={setBody} disabled={busy} />
      </div>
      <Button type="submit" disabled={busy || uploading || !body.trim()}>Send comment</Button>
    </form>
  )

  return (
    <section className="task-comments">
      <div className="task-section-title">Comments <span>{comments.length}</span></div>
      {openWaits.length > 0 && (
        <div className="task-waits">
          Waiting for {openWaits.map((wait) => wait.expected_principal).join(", ")}
        </div>
      )}
      {formFirst ? [commentForm, commentList] : [commentList, commentForm]}
    </section>
  )
}
