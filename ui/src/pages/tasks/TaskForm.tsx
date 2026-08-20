import { useState } from "react"
import type { TaskQueue } from "@/lib/tasks"
import { Input } from "@/components/ui/input"

export default function TaskForm({
  queues,
  initialQueue,
  parentKey = "",
  onCreate,
  onCancel,
}: {
  queues: TaskQueue[]
  initialQueue: string
  parentKey?: string
  onCreate: (input: { queue: string; parent_key: string; title: string }) => Promise<void>
  onCancel: () => void
}) {
  const [queue, setQueue] = useState(initialQueue || queues[0]?.prefix || "")
  const [title, setTitle] = useState("")
  const [busy, setBusy] = useState(false)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!queue || !title.trim()) return
    setBusy(true)
    try {
      await onCreate({ queue, parent_key: parentKey, title: title.trim() })
      setTitle("")
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="task-inline-form" onSubmit={(event) => void submit(event)}>
      {!parentKey && (
        <select aria-label="Task queue" value={queue} onChange={(event) => setQueue(event.target.value)}>
          {queues.map((item) => (
            <option key={item.prefix} value={item.prefix}>{item.prefix}</option>
          ))}
        </select>
      )}
      <Input
        autoFocus
        aria-label="Task title"
        placeholder={parentKey ? `Child of ${parentKey}` : "What needs to be done?"}
        value={title}
        onChange={(event) => setTitle(event.target.value)}
      />
      <button type="submit" disabled={busy || !queue || !title.trim()}>Create task</button>
      <button type="button" onClick={onCancel}>Cancel</button>
    </form>
  )
}
