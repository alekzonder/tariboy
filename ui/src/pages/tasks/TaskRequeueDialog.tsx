import { useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { TaskQueue } from "@/lib/api";
import { cn } from "@/lib/utils";
import { LABEL, MONO, SelectShell } from "./panelStyles";

/**
 * Moving a task to another queue on the same server. A key belongs to its
 * queue, so the daemon creates a new task there and cancels this one; the
 * dialog says so, and says that a workflow stops, before the customer agrees.
 */
export default function TaskRequeueDialog({
  taskKey,
  queue,
  workflow,
  open,
  onOpenChange,
  onRequeue,
}: {
  taskKey: string;
  queue: string;
  workflow: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onRequeue: (queue: string) => Promise<void>;
}) {
  const api = useApi();
  const [queues, setQueues] = useState<TaskQueue[]>([]);
  const [choice, setChoice] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    let live = true;
    api.tasks.listTaskQueues().then(
      (result) => {
        if (live) setQueues(result.queues.filter((item) => item.prefix !== queue));
      },
      (cause) => {
        if (live) setError(cause instanceof Error ? cause.message : String(cause));
      },
    );
    return () => {
      live = false;
    };
  }, [open, queue, api]);

  const requeue = async () => {
    if (!choice) return;
    setBusy(true);
    setError("");
    try {
      await onRequeue(choice);
      onOpenChange(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!busy) onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-[420px]">
        <DialogHeader>
          <DialogTitle>Move to another queue</DialogTitle>
          <DialogDescription>
            <span className={cn(MONO, "text-foreground")}>{taskKey}</span> moves as a new task with
            a new key, and enters the workflow of the chosen queue if it has one.
            {workflow &&
              " The current workflow stops: its scripts are stopped and its questions are closed."}{" "}
            The task here is cancelled and keeps its history.
          </DialogDescription>
        </DialogHeader>
        <label className="flex min-w-0 flex-col gap-1.5">
          <span className={LABEL}>Target queue</span>
          <SelectShell
            aria-label="Target queue"
            value={choice}
            disabled={busy}
            onChange={(event) => setChoice(event.target.value)}
          >
            <option value="">Choose a queue</option>
            {queues.map((item) => (
              <option key={item.prefix} value={item.prefix}>
                {item.prefix} · {item.name}
              </option>
            ))}
          </SelectShell>
        </label>
        {error && (
          <p role="alert" className="text-[12.5px] text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button variant="ghost" size="sm" disabled={busy} onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button size="sm" disabled={busy || !choice} onClick={() => void requeue()}>
            {busy ? "Moving…" : "Move task"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
