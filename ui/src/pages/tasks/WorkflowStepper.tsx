import { Check, Undo2 } from "lucide-react"
import { useState } from "react"
import { StatusDot } from "@/components/ui/status"
import type { WorkflowStatusInfo, WorkflowView } from "@/lib/api"
import { cn } from "@/lib/utils"
import { LABEL, MONO } from "./panelStyles"
import { stepMarks, type StepMark } from "./workflowGraph"

/**
 * The statuses of the pinned image in manifest order, with where the task is
 * and where it has been. A step opens to show what the status needs: its
 * owner, its instructions, the script that owns it, and each way out with
 * the artifacts and checks it requires. It wraps, so a phone never scrolls
 * sideways. A parent that passes `selected` and `onSelect` owns which step is
 * open; the workflow panel filters the artifacts by it.
 */
export default function WorkflowStepper({ view, selected, onSelect }: {
  view: WorkflowView
  selected?: string | null
  onSelect?: (id: string | null) => void
}) {
  const [own, setOwn] = useState<string | null>(null)
  const open = selected === undefined ? own : selected
  const setOpen = onSelect ?? setOwn
  const marks = stepMarks(view)
  const opened = marks.find((mark) => mark.status.id === open)
  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <span className={LABEL}>Workflow</span>
      <ol aria-label="Workflow steps" className="flex min-w-0 flex-wrap items-center gap-x-1 gap-y-1.5">
        {marks.map((mark, index) => (
          <li key={mark.status.id} className="flex min-w-0 items-center gap-1">
            {index > 0 && <span aria-hidden="true" className="h-px w-3 shrink-0 bg-border" />}
            <Step mark={mark} open={open === mark.status.id} onToggle={() => setOpen(open === mark.status.id ? null : mark.status.id)} />
          </li>
        ))}
      </ol>
      {opened && <StepDetail status={opened.status} />}
    </section>
  )
}

function Step({ mark, open, onToggle }: { mark: StepMark; open: boolean; onToggle: () => void }) {
  const { status, state, returns } = mark
  const tone = state === "current" ? (status.terminal ? "quiet" : "live") : state === "visited" ? "quiet" : "faint"
  return (
    <button type="button" aria-current={state === "current" ? "step" : undefined} aria-expanded={open} aria-pressed={open} data-state={state}
      onClick={onToggle}
      className={cn("flex min-w-0 max-w-full items-center gap-1.5 rounded-[7px] px-1.5 py-0.5 text-left hover:bg-accent",
        state === "current" && "bg-muted", open && "ring-1 ring-ring", state === "ahead" && "text-muted-foreground opacity-70")}>
      {state === "visited"
        ? <Check aria-hidden="true" className="size-3 shrink-0 text-status-running [stroke-width:2]" />
        : status.terminal
          ? <span aria-hidden="true" className="size-[7px] shrink-0 rounded-[1.5px] bg-status-stopped" />
          : <StatusDot tone={tone} />}
      <span className={cn(MONO, "min-w-0 max-w-[18ch] truncate", state === "current" && "font-medium text-foreground")} title={status.id}>{status.id}</span>
      {returns > 0 && <span className={cn(MONO, "inline-flex items-center gap-0.5 text-muted-foreground")}
        title={`The task came back to this status ${returns === 1 ? "once" : `${returns} times`}`}>
        <Undo2 aria-hidden="true" className="size-2.5" />{returns}
      </span>}
    </button>
  )
}

/** The owner of a status as a short phrase. */
function ownerText(status: WorkflowStatusInfo): string {
  if (status.terminal) return status.cancelled ? "terminal, counts as cancelled" : "terminal"
  if (status.owner.startsWith("pool:")) return `pool ${status.owner.slice(5)}`
  if (status.owner === "customer") return "the customer"
  if (status.owner === "script") return "a watch script"
  return status.owner
}

function StepDetail({ status }: { status: WorkflowStatusInfo }) {
  return (
    <div data-testid="step-detail" className="flex min-w-0 flex-col gap-1.5 rounded-[8px] bg-muted/50 px-2.5 py-2 text-[12px]">
      <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5">
        <span className={cn(MONO, "min-w-0 break-all font-medium")}>{status.id}</span>
        <span className="text-muted-foreground">owned by {ownerText(status)}{status.initial ? ", where a new task starts" : ""}</span>
        {status.instructions_path && <span className={cn(MONO, "min-w-0 break-all text-muted-foreground")}>{status.instructions_path}</span>}
      </div>
      {status.watch && <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <span className="text-muted-foreground">watch</span>
        <span className={cn(MONO, "min-w-0 break-all")}>{status.watch.script}</span>
        {status.watch.description && <span className="min-w-0 break-words">{status.watch.description}</span>}
        <span className={cn(MONO, "text-muted-foreground")}>every {status.watch.every}, timeout {status.watch.timeout}</span>
      </div>}
      {(status.transitions ?? []).length > 0 && <ul className="flex min-w-0 flex-col gap-1">
        {(status.transitions ?? []).map((transition) => (
          <li key={transition.on} className="flex min-w-0 flex-col gap-0.5">
            <span className="flex min-w-0 flex-wrap items-center gap-x-1.5">
              <span className={cn(MONO, "font-medium")}>{transition.on}</span>
              <span className="text-muted-foreground">→</span>
              <span className={cn(MONO, "min-w-0 break-all")}>{transition.to}</span>
              {(transition.requires ?? []).length > 0 && <span className="text-muted-foreground">
                requires {(transition.requires ?? []).map((name, i) => <span key={name}>{i > 0 ? ", " : ""}<span className={MONO}>{name}</span></span>)}
              </span>}
            </span>
            {(transition.checks ?? []).map((check) => (
              <span key={check.script} className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5 pl-3">
                <span className="text-muted-foreground">check</span>
                <span className={cn(MONO, "min-w-0 break-all")}>{check.script}</span>
                {check.description && <span className="min-w-0 break-words">{check.description}</span>}
                <span className={cn(MONO, "text-muted-foreground")}>as {check.run_as}, timeout {check.timeout}</span>
              </span>
            ))}
          </li>
        ))}
      </ul>}
      {status.limits && <span className={cn(MONO, "text-muted-foreground")}>
        pauses after {status.limits.idle_iterations} idle iterations, {status.limits.rejected_requests} rejections, or {status.limits.script_failures} script failures in a row; holder grace {status.limits.unavailable_grace}
      </span>}
    </div>
  )
}
