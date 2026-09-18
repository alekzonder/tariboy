import { Inbox, ListTree, UserRound, UsersRound } from "lucide-react"
import type { TaskQueue } from "@/lib/tasks"

export type TasksView = "all" | "mine" | "waiting" | "queues"

const ITEMS = [
  { key: "all", label: "All tasks", icon: ListTree },
  { key: "mine", label: "My tasks", icon: UserRound },
  { key: "waiting", label: "Waiting for me", icon: Inbox },
  { key: "queues", label: "Queues", icon: UsersRound },
] as const

export default function TasksNavigation({
  view,
  onView,
  queues,
  queue,
  onQueue,
}: {
  view: TasksView
  onView: (view: TasksView) => void
  queues: TaskQueue[]
  queue: string
  onQueue: (prefix: string) => void
}) {
  return (
    <aside className="tasks-navigation">
      <div className="tasks-brand">
        <h1>Tasks</h1>
        <span>Native workspace</span>
      </div>
      <nav aria-label="Task views">
        {ITEMS.map(({ key, label, icon: Icon }) => (
          <button
            type="button"
            key={key}
            className={view === key ? "is-active" : ""}
            onClick={() => onView(key)}
          >
            <Icon aria-hidden="true" />
            <span>{label}</span>
          </button>
        ))}
      </nav>
      <label className="tasks-queue-picker">
        Queue
        <select value={queue} onChange={(event) => onQueue(event.target.value)}>
          <option value="">All queues</option>
          {queues.map((item) => (
            <option key={item.prefix} value={item.prefix}>
              {item.prefix} · {item.name}
            </option>
          ))}
        </select>
      </label>
    </aside>
  )
}
