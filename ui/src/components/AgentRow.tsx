import type { ComponentProps } from "react"

import { StatusDot, StatusPill } from "@/components/ui/status"
import { agentTone } from "@/lib/statusTone"
import { cn } from "@/lib/utils"

/**
 * One agent in the sidebar. 30px tall, and the selected one is the same
 * "island" as the content panel — `--card` lifted by `--raise` — so the
 * selection reads as the row the panel belongs to rather than a highlight.
 *
 * It is a plain button: the sidebar wraps it in drag-and-drop and a context
 * menu, and passes their props straight through.
 */
export function AgentRow({
  name,
  state,
  outOfBudget = false,
  selected = false,
  unreadMessages = 0,
  interactive = true,
  className,
  ...props
}: ComponentProps<"button"> & {
  name: string
  state: string
  /** Budget exhausted — outranks the state in both the dot and the pill. */
  outOfBudget?: boolean
  selected?: boolean
  /** Unread chat messages from this agent. Zero renders nothing at all. */
  unreadMessages?: number
  /** false → the agent has no tty; shown as a quiet marker after the name. */
  interactive?: boolean
}) {
  const tone = agentTone(state, outOfBudget)
  return (
    <button
      type="button"
      data-slot="agent-row"
      aria-current={selected ? "page" : undefined}
      className={cn(
        "my-px flex h-[30px] w-full items-center gap-[9px] rounded-[8px] px-2.5 text-left text-[13px]",
        selected ? "bg-card shadow-[var(--raise)]" : "hover:bg-sidebar-accent",
        className,
      )}
      {...props}
    >
      <StatusDot tone={tone} />
      <span className="min-w-0 flex-1 truncate font-medium">{name}</span>
      {!interactive && (
        <span className="shrink-0 text-[11px] text-muted-foreground" title="not interactive (no tty)">
          non-tty
        </span>
      )}
      {unreadMessages > 0 && (
        <span
          aria-label={`${unreadMessages} unread messages from ${name}`}
          title={`${unreadMessages} unread messages`}
          className="shrink-0 rounded-full bg-primary px-1.5 text-[10px] leading-4 font-medium text-primary-foreground"
        >
          {unreadMessages > 99 ? "99+" : unreadMessages}
        </span>
      )}
      {outOfBudget ? (
        <StatusPill tone="danger" size="sm" title="Budget exhausted — agent paused until the cap is raised">
          no budget
        </StatusPill>
      ) : (
        <StatusPill tone={tone} size="sm" quiet="chip" title={`Status: ${state}`}>
          {state}
        </StatusPill>
      )}
    </button>
  )
}
