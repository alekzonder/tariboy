import { useMemo, useState } from "react";
import type { ApiTarget } from "@/lib/api";
import type { DaemonMeta } from "@/lib/daemons";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export function eligibleImageTransferTargets(source: ApiTarget, daemons: DaemonMeta[]): DaemonMeta[] {
  return daemons.filter((host) => host.state === "ready" && (source === null || source === undefined || host.id !== source.id));
}

interface ImageTransferDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  source: ApiTarget;
  ref: string;
  daemons: DaemonMeta[];
  onComplete: () => void;
}

export function ImageTransferDialog({
  open,
  onOpenChange,
  source,
  ref,
  daemons,
}: ImageTransferDialogProps) {
  const targets = useMemo(
    () => eligibleImageTransferTargets(source, daemons),
    [source, daemons],
  );
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const selectedCount = targets.filter((target) => selected.has(target.id)).length;
  const allSelected = targets.length > 0 && selectedCount === targets.length;

  const toggleAll = () => {
    setSelected(allSelected ? new Set() : new Set(targets.map((target) => target.id)));
  };

  const toggleTarget = (id: string) => {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Transfer image {ref}</DialogTitle>
          <DialogDescription>
            Select the ready servers that should receive this runnable image.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          {targets.length === 0 ? (
            <p className="text-sm text-muted-foreground">No ready servers are available for transfer.</p>
          ) : (
            <>
              <label className="flex items-center gap-2 text-sm font-medium">
                <input
                  type="checkbox"
                  aria-label="Select all servers"
                  checked={allSelected}
                  onChange={toggleAll}
                />
                Select all servers
              </label>
              <ul className="space-y-2" aria-label="Eligible transfer targets">
                {targets.map((target) => (
                  <li key={target.id}>
                    <label className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        aria-label={`Transfer to ${target.label}`}
                        checked={selected.has(target.id)}
                        onChange={() => toggleTarget(target.id)}
                      />
                      {target.label}
                    </label>
                  </li>
                ))}
              </ul>
            </>
          )}
          <p aria-live="polite" className="text-sm text-muted-foreground">
            {targets.length === 0 ? "No transfer targets selected." : `${selectedCount} transfer target${selectedCount === 1 ? "" : "s"} selected.`}
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button disabled={selectedCount === 0}>Start transfer</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
