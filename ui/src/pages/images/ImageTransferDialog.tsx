import { useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { applyImageArchiveOn, downloadImageArchiveOn, uploadImageArchiveOn } from "@/lib/teamApi";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export interface ImageTransferTarget {
  id: string;
  label: string;
  target: Daemon | null;
}

const localTarget: ImageTransferTarget = {
  id: "",
  label: "This daemon (local)",
  target: null,
};

export function eligibleImageTransferTargets(source: Daemon | null, daemons: Daemon[]): ImageTransferTarget[] {
  return [
    ...(source === null ? [] : [localTarget]),
    ...daemons
      .filter((host) => host.id !== "" && host.state === "ready" && (source === null || host.id !== source.id))
      .map((host) => ({ id: host.id, label: host.label, target: host })),
  ];
}

interface ImageTransferDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  source: Daemon | null;
  ref: string;
  daemons: Daemon[];
  onComplete: () => void;
}

type TransferStatus = "queued" | "exporting" | "previewing" | "importing" | "completed" | "already-present" | "failed" | "cancelled";
interface TransferRow {
  status: TransferStatus;
  importID?: string;
  error?: string;
  retryRef?: string;
}

const errorMessage = (error: unknown) => error instanceof Error ? error.message : String(error);
const isConflict = (error: unknown) => error instanceof ApiError && error.status === 409;
const completedRows = (rows: Record<string, TransferRow>) => Object.values(rows)
  .filter((row) => row.status === "completed" || row.status === "already-present").length;

export function ImageTransferDialog({
  open,
  onOpenChange,
  source,
  ref,
  daemons,
  onComplete,
}: ImageTransferDialogProps) {
  const discoveredTargets = useMemo(
    () => eligibleImageTransferTargets(source, daemons),
    [source, daemons],
  );
  const [openTargets, setOpenTargets] = useState<ImageTransferTarget[]>(discoveredTargets);
  const [runTargets, setRunTargets] = useState<ImageTransferTarget[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [rows, setRows] = useState<Record<string, TransferRow>>({});
  const [phase, setPhase] = useState<"idle" | "exporting" | "transferring">("idle");
  const archive = useRef<Blob | null>(null);
  const cancelRequested = useRef(false);
  const mounted = useRef(true);
  const transfer = useRef(0);
  const previouslyOpen = useRef(false);
  const targets = runTargets ?? openTargets;
  const exporting = phase === "exporting";
  const transferring = phase === "transferring";
  const controlsDisabled = phase !== "idle";
  const selectedCount = targets.filter((target) => selected.has(target.id)).length;
  const allSelected = targets.length > 0 && selectedCount === targets.length;

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      cancelRequested.current = true;
      archive.current = null;
    };
  }, []);

  useEffect(() => {
    if (open) {
      if (!previouslyOpen.current) setOpenTargets(discoveredTargets);
      previouslyOpen.current = true;
      return;
    }
    previouslyOpen.current = false;
    cancelRequested.current = true;
    transfer.current += 1;
    archive.current = null;
    setRows({});
    setSelected(new Set());
    setRunTargets(null);
    setPhase("idle");
  }, [discoveredTargets, open]);

  const setRow = (id: string, row: TransferRow, run: number) => {
    if (mounted.current && transfer.current === run) setRows((current) => ({ ...current, [id]: row }));
  };

  const cancel = () => {
    if (exporting) return;
    cancelRequested.current = true;
  };

  const close = () => {
    cancel();
    transfer.current += 1;
    archive.current = null;
    setRows({});
    setSelected(new Set());
    setRunTargets(null);
    setPhase("idle");
    onOpenChange(false);
  };

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

  const start = async () => {
    const selectedTargets = targets.filter((target) => selected.has(target.id));
    if (selectedTargets.length === 0) return;
    const run = ++transfer.current;
    cancelRequested.current = false;
    setRunTargets(targets);
    setPhase("exporting");
    setRows(Object.fromEntries(selectedTargets.map((target) => [target.id, { status: "exporting" }])));
    let exportedArchive: Blob;
    try {
      exportedArchive = await downloadImageArchiveOn(source, ref);
      if (!mounted.current || transfer.current !== run) return;
      archive.current = exportedArchive;
    } catch (error) {
      const failure = { status: "failed" as const, error: errorMessage(error) };
      if (mounted.current && transfer.current === run) setRows(Object.fromEntries(selectedTargets.map((target) => [target.id, failure])));
      if (mounted.current && transfer.current === run) setPhase("idle");
      return;
    }

    if (mounted.current && transfer.current === run) {
      setRows(Object.fromEntries(selectedTargets.map((target) => [target.id, { status: "queued" }])));
      setPhase("transferring");
    }

    for (let index = 0; index < selectedTargets.length; index += 1) {
      const target = selectedTargets[index];
      if (transfer.current !== run) return;
      if (cancelRequested.current) {
        for (const remaining of selectedTargets.slice(index)) setRow(remaining.id, { status: "cancelled" }, run);
        break;
      }
      try {
        setRow(target.id, { status: "previewing" }, run);
        const preview = await uploadImageArchiveOn(target.target, exportedArchive);
        if (transfer.current !== run) return;
        setRow(target.id, { status: "importing", importID: preview.import_id }, run);
        const result = await applyImageArchiveOn(target.target, preview.import_id, ref) as { reused?: boolean };
        setRow(target.id, { status: result?.reused ? "already-present" : "completed" }, run);
      } catch (error) {
        setRow(target.id, {
          status: "failed",
          error: errorMessage(error),
          ...(isConflict(error) ? { retryRef: ref } : {}),
        }, run);
      }
    }
    if (mounted.current && transfer.current === run) {
      setPhase("idle");
      if (!cancelRequested.current) onComplete();
    }
  };

  const retry = async (targetID: string) => {
    const target = runTargets?.find((candidate) => candidate.id === targetID);
    const retryRef = rows[targetID]?.retryRef;
    const exportedArchive = archive.current;
    if (!target || !retryRef || !exportedArchive) return;
    const run = ++transfer.current;
    setPhase("transferring");
    try {
      setRow(targetID, { status: "previewing" }, run);
      const preview = await uploadImageArchiveOn(target.target, exportedArchive);
      if (transfer.current !== run) return;
      setRow(targetID, { status: "importing", importID: preview.import_id }, run);
      const result = await applyImageArchiveOn(target.target, preview.import_id, retryRef) as { reused?: boolean };
      setRow(targetID, { status: result?.reused ? "already-present" : "completed" }, run);
      if (mounted.current && transfer.current === run) onComplete();
    } catch (error) {
      setRow(targetID, {
        status: "failed",
        error: errorMessage(error),
        ...(isConflict(error) ? { retryRef } : {}),
      }, run);
    } finally {
      if (mounted.current && transfer.current === run) setPhase("idle");
    }
  };

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => {
      if (!nextOpen) {
        if (!exporting) close();
        return;
      }
      onOpenChange(true);
    }}>
      <DialogContent
        showCloseButton={!exporting}
        onEscapeKeyDown={(event) => { if (exporting) event.preventDefault(); }}
        onPointerDownOutside={(event) => { if (exporting) event.preventDefault(); }}
      >
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
              <Button variant="outline" size="sm" onClick={toggleAll} disabled={controlsDisabled}>
                {allSelected ? "Clear all servers" : "All servers"}
              </Button>
              <ul className="space-y-2" aria-label="Eligible transfer targets">
                {targets.map((target) => (
                  <li key={target.id}>
                    <label className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        aria-label={`Transfer to ${target.label}`}
                        checked={selected.has(target.id)}
                        onChange={() => toggleTarget(target.id)}
                        disabled={controlsDisabled}
                      />
                      {target.label}
                    </label>
                  </li>
                ))}
              </ul>
              <ul className="space-y-2" aria-label="Transfer progress">
                {targets.filter((target) => rows[target.id]).map((target) => {
                  const row = rows[target.id];
                  const label = row.status === "already-present" ? "Already present" : row.status[0].toUpperCase() + row.status.slice(1);
                  return <li key={target.id} className="text-sm">
                    <p>{target.label}: {label}{row.error ? ` — ${row.error}` : ""}</p>
                    {row.retryRef !== undefined && <div className="mt-1 flex gap-2">
                      <Input aria-label={`Retag and retry for ${target.label}`} value={row.retryRef} disabled={controlsDisabled}
                        onChange={(event) => setRows((current) => ({ ...current, [target.id]: { ...current[target.id], retryRef: event.target.value } }))} />
                      <Button size="sm" onClick={() => void retry(target.id)} disabled={controlsDisabled || !row.retryRef.trim()}>
                        Retag and retry {target.label}
                      </Button>
                    </div>}
                  </li>;
                })}
              </ul>
            </>
          )}
          <p aria-live="polite" className="text-sm text-muted-foreground">
            {exporting ? "Exporting source archive." : transferring ? `Transferring image: ${completedRows(rows)} target${completedRows(rows) === 1 ? "" : "s"} completed.` : targets.length === 0 ? "No transfer targets selected." : `${selectedCount} transfer target${selectedCount === 1 ? "" : "s"} selected.`}
          </p>
        </div>
        <DialogFooter>
          {phase !== "idle" ? <Button variant="outline" onClick={cancel} disabled={exporting}>Cancel transfer</Button> : <Button variant="outline" onClick={close}>Cancel</Button>}
          <Button disabled={selectedCount === 0 || controlsDisabled} onClick={() => void start()}>Start transfer</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
