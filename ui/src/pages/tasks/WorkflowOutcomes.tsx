import { ArrowRight, Check } from "lucide-react"
import { useState } from "react"
import { useApi } from "@/components/apiContext"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { TransitionRequest, WorkflowOutcome, WorkflowView } from "@/lib/api"
import { cn } from "@/lib/utils"
import { DANGER_FILL, EMPTY, LABEL, MONO } from "./panelStyles"
import { useFollowRequest } from "./workflowShared"

const RESULT_LABEL: Record<string, string> = { rejected: "Rejected", failed: "Failed", cancelled: "Cancelled" }

type Notice = { kind: "result"; request: TransitionRequest } | { kind: "error"; label: string; message: string } | { kind: "timeout"; requestId: number }

/**
 * The outcomes of the current status: each with its target, the artifacts it
 * requires (set or still missing), and its checks with what they verify. The
 * Desktop acts as the customer, so a customer status gets one button per
 * outcome; a pool or script status is listed read-only, since its holder or
 * its watch script decides. A request that settled without applying is
 * explained by the problem block once the view carries it.
 */
export default function WorkflowOutcomes({ taskKey, view, onChanged, onRefresh }: {
  taskKey: string
  view: WorkflowView
  /** The transition applied: refetch the view and the task. */
  onChanged: () => void
  /** Refetch the view only. */
  onRefresh: () => void
}) {
  const api = useApi()
  const customer = view.owner === "customer"
  const paused = view.waiting_on === "pause"
  const [message, setMessage] = useState("")
  const { checking, follow } = useFollowRequest(taskKey)
  // A notice belongs to the status it was given in; once the view shows
  // another status it no longer applies and is not shown.
  const [held, setHeld] = useState<{ status: string; notice: Notice } | null>(null)
  const notice = held?.status === view.status ? held.notice : null
  const setNotice = (next: Notice | null, status = view.status) => setHeld(next && { status, notice: next })

  const advance = (outcome: string) => {
    setNotice(null)
    void follow(() => api.workflows.advanceTask(taskKey, outcome, message, view.status), (result) => {
      switch (result.kind) {
        case "applied": setMessage(""); onChanged(); return
        case "result": setNotice({ kind: "result", request: result.request }); onRefresh(); return
        case "timeout": setNotice({ kind: "timeout", requestId: result.requestId }); return
        case "error":
          // status_changed names the status the task is in now, where the
          // refetched view lands, so its message survives that refetch.
          setNotice({ kind: "error", label: result.label, message: result.message }, result.status ?? view.status)
          if (result.stale) onRefresh()
      }
    })
  }

  const last = view.last_request
  // A notice about a request the view now carries is the problem block's to
  // explain; one about a request the view has not caught up with stays.
  const shown: Notice | null = notice && (
    (notice.kind === "timeout" && last?.id === notice.requestId && last.state !== "pending") ||
    (notice.kind === "result" && last?.id === notice.request.id)) ? null : notice
  const pendingElsewhere = !checking && last?.state === "pending"
  const disabled = checking || paused || pendingElsewhere

  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <span className={LABEL}>Outcomes</span>
      {view.outcomes.length === 0 && <span className={EMPTY}>This status has no outcomes.</span>}
      {!customer && view.outcomes.length > 0 && <span className={EMPTY}>
        {view.owner === "script" ? "The status's watch script decides." : "The agent decides; the outcomes are listed for reference."}
      </span>}
      <ul className="flex min-w-0 flex-col gap-1.5">
        {view.outcomes.map((outcome) => {
          const missing = new Set(outcome.missing ?? [])
          return (
            <li key={outcome.on} className="flex min-w-0 flex-col gap-0.5">
              <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                {customer
                  ? <Button type="button" size="sm" className="h-7" disabled={disabled || missing.size > 0}
                    onClick={() => advance(outcome.on)}>{outcome.on}</Button>
                  : <span className={cn(MONO, "font-medium")}>{outcome.on}</span>}
                {customer && missing.size > 0 && <span className="text-[11.5px] text-status-failed">needs: {[...missing].join(", ")}</span>}
                <ArrowRight aria-hidden="true" className="size-3 text-muted-foreground" />
                <span className={cn(MONO, "min-w-0 break-all")}>{outcome.to}</span>
                {(outcome.requires ?? []).length > 0 && <span className="text-[11.5px] text-muted-foreground">requires</span>}
                {(outcome.requires ?? []).map((name) => missing.has(name)
                  ? <span key={name} data-missing="true" title="No value yet"
                    className={cn(MONO, "rounded-[5px] px-1.5", DANGER_FILL)}>{name}</span>
                  : <span key={name} data-present="true" title="Set" className={cn(MONO, "inline-flex items-center gap-0.5")}>
                    <Check aria-hidden="true" className="size-2.5 text-status-running [stroke-width:2.5]" />{name}</span>)}
              </div>
              <Checks outcome={outcome} />
            </li>
          )
        })}
      </ul>
      {customer && view.outcomes.length > 0 && <Textarea aria-label="Outcome message" placeholder="Message (optional)"
        value={message} disabled={checking} maxLength={4096} onChange={(event) => setMessage(event.target.value)}
        className="min-h-12 rounded-[8px] border-0 bg-muted text-[12.5px] md:text-[12.5px]" />}
      {(checking || pendingElsewhere) && <div className="flex items-center gap-2">
        <span role="status" className={EMPTY}>checking…</span>
        {pendingElsewhere && <Button type="button" variant="ghost" className="h-6 px-2 text-[12px]" onClick={onRefresh}>Refresh</Button>}
      </div>}
      {shown && <OutcomeNotice notice={shown} onRefresh={onRefresh} />}
    </section>
  )
}

/** The checks of an outcome: each script with what it verifies, when the image says. */
function Checks({ outcome }: { outcome: WorkflowOutcome }) {
  const checks = outcome.checks ?? []
  if (checks.length === 0) return null
  return <ul className="flex min-w-0 flex-col gap-0.5 pl-3">
    {checks.map((script, index) => {
      const detail = outcome.check_details?.[index]
      return <li key={script} className="flex min-w-0 flex-wrap items-baseline gap-x-2 text-[11.5px]">
        <span className="text-muted-foreground">check</span>
        <span className={cn(MONO, "min-w-0 break-all text-muted-foreground")}>{script}</span>
        {detail?.description && <span className="min-w-0 break-words">{detail.description}</span>}
        {detail && <span className={cn(MONO, "text-muted-foreground opacity-80")}>as {detail.run_as}, timeout {detail.timeout}</span>}
      </li>
    })}
  </ul>
}

function OutcomeNotice({ notice, onRefresh }: { notice: Notice; onRefresh: () => void }) {
  if (notice.kind === "timeout") {
    return <div role="status" className="flex items-center gap-2 text-[12px] text-muted-foreground">
      Still checking — the result appears here when the checks finish.
      <Button type="button" variant="ghost" className="h-6 px-2 text-[12px]" onClick={onRefresh}>Refresh</Button>
    </div>
  }
  const text = notice.kind === "error" ? notice.message : notice.request.result_message
  const label = notice.kind === "error" ? notice.label : RESULT_LABEL[notice.request.state] ?? notice.request.state
  return <div role="alert" className={cn("flex min-w-0 flex-col gap-1 rounded-[8px] px-2.5 py-2 text-[12px]", DANGER_FILL)}>
    <span className="font-medium">{label}{notice.kind === "result" ? ` · ${notice.request.outcome}` : ""}</span>
    {text && <pre className="min-w-0 font-mono text-[11.5px] whitespace-pre-wrap break-words">{text}</pre>}
  </div>
}
