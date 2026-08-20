import type { TaskNotification } from "@/lib/tasks"

export default function TaskNotifications({
  notifications,
  onOpen,
  onRead,
  onDismiss,
}: {
  notifications: TaskNotification[]
  onOpen: (key: string) => void
  onRead: (id: string) => Promise<void>
  onDismiss: (id: string) => Promise<void>
}) {
  return (
    <section className="task-inbox">
      <header><h2>Notifications</h2><span>{notifications.filter((item) => !item.read_at).length} unread</span></header>
      {notifications.map((item) => (
        <article key={item.id} className={item.read_at ? "is-read" : ""}>
          <button type="button" className="task-notification-main" onClick={() => onOpen(item.task_key)}>
            <strong>{item.task_key}</strong>
            <span>{item.text}</span>
            <time>{new Date(item.created_at).toLocaleString()}</time>
          </button>
          <div>
            {!item.read_at && <button type="button" onClick={() => void onRead(item.id)}>Mark read</button>}
            <button type="button" onClick={() => void onDismiss(item.id)}>Dismiss</button>
          </div>
        </article>
      ))}
      {notifications.length === 0 && <div className="tasks-empty">Inbox zero.</div>}
    </section>
  )
}
