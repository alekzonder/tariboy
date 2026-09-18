import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useOptionalDaemons } from "@/components/DaemonProvider"
import { cn } from "@/lib/utils"
import { LABEL, MONO, SelectShell } from "./panelStyles"

/**
 * Moving a task to another server is the one task action that needs a second
 * host, so the dialog is a host picker and nothing else. The queue is stated
 * rather than chosen: the target daemon must already run a queue with the same
 * prefix, and it says so itself if it does not.
 */
export default function TaskTransferDialog({
  taskKey,
  queue,
  open,
  onOpenChange,
  onTransfer,
}: {
  taskKey: string
  queue: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onTransfer: (hostID: string) => Promise<void>
}) {
  // The registry is optional: a panel can be mounted outside the desktop shell,
  // and then there is no second host to move anything to. The current host is
  // not a destination either, so a transfer needs at least one other row.
  const registry = useOptionalDaemons()
  const targets = (registry?.daemons ?? []).filter((host) => host.id !== registry?.activeId)
  const [hostID, setHostID] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  const transfer = async () => {
    if (!hostID) return
    setBusy(true)
    setError("")
    try {
      await onTransfer(hostID)
      onOpenChange(false)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!busy) onOpenChange(next) }}>
      <DialogContent className="sm:max-w-[420px]">
        <DialogHeader>
          <DialogTitle>Move to another server</DialogTitle>
          <DialogDescription>
            <span className={cn(MONO, "text-foreground")}>{taskKey}</span> and everything under it
            move to the chosen server, which must already run a <span className={MONO}>{queue}</span> queue.
            The task here is cancelled and keeps its history.
          </DialogDescription>
        </DialogHeader>
        {targets.length === 0 ? (
          <p className="text-[12.5px] text-muted-foreground">
            No other server is registered. Add a host first.
          </p>
        ) : (
          <label className="flex min-w-0 flex-col gap-1.5">
            <span className={LABEL}>Target server</span>
            <SelectShell aria-label="Target server" value={hostID} disabled={busy}
              onChange={(event) => setHostID(event.target.value)}>
              <option value="">Choose a server</option>
              {targets.map((host) => (
                <option key={host.id} value={host.id}>{host.label || host.id}</option>
              ))}
            </SelectShell>
          </label>
        )}
        {error && <p role="alert" className="text-[12.5px] text-destructive">{error}</p>}
        <DialogFooter>
          <Button variant="ghost" size="sm" disabled={busy} onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button size="sm" disabled={busy || !hostID} onClick={() => void transfer()}>
            {busy ? "Moving…" : "Move task"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
