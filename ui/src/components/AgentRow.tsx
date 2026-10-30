import type { ComponentProps } from "react";

import { StatusDot, StatusPill } from "@/components/ui/status";
import { agentStateTone, type AgentState } from "@/lib/agentState";
import { cn } from "@/lib/utils";
import { elapsed } from "@/pages/home/homeModel";

/** The text on the right of a row; an idle agent has none, so a long list
 *  reads as the few agents to look at. */
function pillText(state: AgentState, now: Date): string {
  switch (state.kind) {
    case "needs":
      return `needs you · ${state.needs.count}`;
    case "working": {
      const time = state.since ? elapsed(state.since, now) : "";
      return time ? `working · ${time}` : "working";
    }
    case "error":
      return "error";
    case "off":
      return "off";
    default:
      return "";
  }
}

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
  agentState,
  now,
  selected = false,
  interactive = true,
  className,
  ...props
}: ComponentProps<"button"> & {
  name: string;
  /** The derived state: the dot's color and the pill on the right. */
  agentState: AgentState;
  /** The clock "working · 12 min" counts from. */
  now: Date;
  selected?: boolean;
  /** false → the agent has no tty; shown as a quiet marker after the name. */
  interactive?: boolean;
}) {
  const tone = agentStateTone(agentState);
  const pill = pillText(agentState, now);
  return (
    <button
      type="button"
      data-slot="agent-row"
      aria-current={selected ? "page" : undefined}
      className={cn(
        "my-px flex h-7.5 w-full items-center gap-2.25 rounded-md px-2.5 text-left text-body",
        selected ? "bg-card shadow-[var(--raise)]" : "hover:bg-sidebar-accent",
        className,
      )}
      {...props}
    >
      <StatusDot tone={tone} />
      <span className="min-w-0 flex-1 truncate font-medium">{name}</span>
      {!interactive && (
        <span
          className="shrink-0 text-caption text-muted-foreground"
          title="not interactive (no tty)"
        >
          non-tty
        </span>
      )}
      {pill && (
        <StatusPill
          data-slot="agent-row-pill"
          tone={tone}
          size="sm"
          title={agentState.kind === "error" ? agentState.reason : undefined}
        >
          {pill}
        </StatusPill>
      )}
    </button>
  );
}
