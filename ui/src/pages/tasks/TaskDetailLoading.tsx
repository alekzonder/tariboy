import type { CSSProperties, ReactNode } from "react"
import { X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { EMPTY } from "./panelStyles"

/**
 * The same sheet while its task is still being read, so a click opens the panel
 * at once. It takes no focus: the element that opened it stays the one the
 * loaded panel returns focus to.
 */
export function TaskDetailLoading({ taskKey, width, resizeHandle, onClose, error }: {
  error?: string
  taskKey: string
  width: number
  resizeHandle: ReactNode
  onClose: () => void
}) {
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
    <DialogContent className="task-detail-dialog bg-card ring-0"
      showCloseButton={false} aria-describedby={undefined}
      overlayClassName="bg-[color-mix(in_oklab,var(--foreground)_7%,transparent)] supports-backdrop-filter:backdrop-blur-none"
      style={{ "--tasks-detail-width": `${width}px` } as CSSProperties}
      onOpenAutoFocus={(event) => event.preventDefault()}
      onCloseAutoFocus={(event) => event.preventDefault()}>
    {resizeHandle}
    <div className="task-detail-panel" aria-busy={!error}>
      <header className="sticky top-0 z-[3] flex items-start gap-2.5 bg-card px-3.5 pt-[11px] pb-2.5 shadow-[var(--raise)]">
        <div className="flex min-w-0 flex-1 items-center gap-[9px]">
          {/* Names the dialog without being the task heading, which marks a loaded task. */}
          <DialogTitle asChild>
            <span className="shrink-0 font-mono text-[12px] font-medium tabular-nums text-muted-foreground">{taskKey}</span>
          </DialogTitle>
        </div>
        <Button variant="ghost" size="icon" aria-label="Close task detail" onClick={onClose}
          className="size-7 shrink-0 rounded-[8px] text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></Button>
      </header>
      <div className="task-detail-body px-4 pt-3.5 pb-7">
        {error ? <p role="alert" className="text-status-failed">{error}</p> : <span className={EMPTY} role="status">Loading task…</span>}
      </div>
    </div>
    </DialogContent>
    </Dialog>
  )
}
