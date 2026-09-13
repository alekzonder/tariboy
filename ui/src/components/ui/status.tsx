import type { ComponentProps } from "react"

import { cn } from "@/lib/utils"
import {
  TONE_DOT,
  TONE_FILL,
  TONE_TEXT,
  priorityTone,
  type StatusTone,
} from "@/lib/statusTone"

/**
 * The status primitives of the style layer. None of them decides what a status
 * means — they take a tone from `@/lib/statusTone` and render it at one of the
 * two densities the handoff uses.
 */

/** 17px chip (sidebar rows, priorities) or 20px pill (task rows, agent header). */
type PillSize = "sm" | "md"

const PILL_SIZE: Record<PillSize, string> = {
  sm: "h-[17px] rounded-[5px] px-1.5 text-[10.5px] tracking-[.01em]",
  md: "h-5 rounded-[6px] px-2 text-[11.5px]",
}

/** The height without the box, so a quiet status still sits on the row's baseline. */
const TEXT_SIZE: Record<PillSize, string> = {
  sm: "h-[17px] text-[10.5px] tracking-[.01em]",
  md: "h-5 text-[11.5px]",
}

export function StatusPill({
  tone,
  size = "md",
  quiet = "text",
  className,
  ...props
}: ComponentProps<"span"> & {
  tone: StatusTone
  size?: PillSize
  /** How a `quiet`/`faint` tone renders: as bare text (default), or as the
   *  neutral chip, for layouts where the shape holds a column. */
  quiet?: "text" | "chip"
}) {
  const filled = tone === "live" || tone === "attention" || tone === "danger" || quiet === "chip"
  return (
    <span
      data-slot="status-pill"
      data-tone={tone}
      className={cn(
        "inline-flex w-fit shrink-0 items-center gap-1.5 whitespace-nowrap",
        filled ? PILL_SIZE[size] : TEXT_SIZE[size],
        filled ? TONE_FILL[tone] : TONE_TEXT[tone],
        className,
      )}
      {...props}
    />
  )
}

/** 7px in a sidebar row, 8px in the agent header. */
export function StatusDot({
  tone,
  size = 7,
  className,
  ...props
}: ComponentProps<"span"> & { tone: StatusTone; size?: 7 | 8 }) {
  return (
    <span
      data-slot="status-dot"
      data-tone={tone}
      style={{ width: size, height: size }}
      className={cn("shrink-0 rounded-full", TONE_DOT[tone], className)}
      {...props}
    />
  )
}

/** P1 is the only rank that earns a fill; the rest are the neutral chip. */
export function PriorityTag({
  priority,
  className,
  ...props
}: Omit<ComponentProps<"span">, "children"> & { priority: string }) {
  return (
    <StatusPill
      tone={priorityTone(priority)}
      size="sm"
      quiet="chip"
      className={cn("min-w-[22px] justify-center px-[5px] font-mono font-medium tabular-nums", className)}
      {...props}
    >
      {priority}
    </StatusPill>
  )
}
