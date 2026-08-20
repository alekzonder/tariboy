import { useState } from "react"
import type { TaskComment, TaskPrincipals, TaskWait } from "@/lib/tasks"
import { Textarea } from "@/components/ui/textarea"

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `comment-${Date.now()}-${Math.random()}`
}

export default function TaskComments({
  comments,
  waits,
  principals,
  onComment,
}: {
  comments: TaskComment[]
  waits: TaskWait[]
  principals: TaskPrincipals | null
  onComment: (body: string, idempotencyKey: string) => Promise<void>
}) {
  const [body, setBody] = useState("")
  const [ask, setAsk] = useState("")
  const [busy, setBusy] = useState(false)
  const openWaits = waits.filter((wait) => !wait.resolved_at)
  const choices = principals
    ? [principals.customer, ...principals.agents].filter(Boolean)
    : []

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!body.trim()) return
    setBusy(true)
    try {
      const principal = ask && !ask.includes(":") ? `agent:${ask}` : ask
      const text = principal ? `@${principal} ${body.trim()}` : body.trim()
      await onComment(text, idempotencyKey())
      setBody("")
      setAsk("")
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="task-comments">
      <div className="task-section-title">Comments <span>{comments.length}</span></div>
      {openWaits.length > 0 && (
        <div className="task-waits">
          Waiting for {openWaits.map((wait) => wait.expected_principal).join(", ")}
        </div>
      )}
      <div className="task-comment-list">
        {comments.map((comment) => (
          <article key={comment.id}>
            <header><strong>{comment.author}</strong><time>{new Date(comment.created_at).toLocaleString()}</time></header>
            <p>{comment.body}</p>
          </article>
        ))}
      </div>
      <form onSubmit={(event) => void submit(event)}>
        <label>
          Ask
          <select aria-label="Ask" value={ask} onChange={(event) => setAsk(event.target.value)}>
            <option value="">No explicit answer needed</option>
            {choices.map((principal) => <option key={principal} value={principal}>{principal}</option>)}
          </select>
        </label>
        <label>
          Comment
          <Textarea aria-label="Comment" value={body} onChange={(event) => setBody(event.target.value)} />
        </label>
        <button type="submit" disabled={busy || !body.trim()}>Send comment</button>
      </form>
    </section>
  )
}
