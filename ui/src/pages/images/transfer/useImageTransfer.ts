import { useEffect, useMemo, useRef, useState } from "react";
import { createApi, type ApiClient } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { useApi } from "@/components/apiContext";
import { errorText } from "@/lib/errors";
import { eligibleImageTransferTargets, type ImageTransferTarget } from "../imageTransferTargets";
import type { TransferPhase, TransferRow } from "./transferStatus";
import { transferToTarget } from "./transferToTarget";

export type DestinationTarget = ImageTransferTarget & { api: ApiClient };

const rowsFor = (targets: DestinationTarget[], row: TransferRow) =>
  Object.fromEntries(targets.map((target) => [target.id, row]));

function useTransferSelection(targets: DestinationTarget[]) {
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

  return { selected, setSelected, selectedCount, allSelected, toggleAll, toggleTarget };
}

export function useImageTransfer({
  imageRef,
  daemons,
  onOpenChange,
  onComplete,
}: {
  imageRef: string;
  daemons: Daemon[];
  onOpenChange: (open: boolean) => void;
  onComplete: () => void;
}) {
  // The source is the host this dialog was opened on; it and every
  // destination client are fixed for the open dialog, so a host switch or a
  // registry change never redirects a running transfer.
  const currentSource = useApi();
  const [source] = useState(currentSource);
  const discoveredTargets = useMemo(
    () => eligibleImageTransferTargets(source.target, daemons),
    [source, daemons],
  );
  const [openTargets] = useState<DestinationTarget[]>(() =>
    discoveredTargets.map((target) => ({ ...target, api: createApi(target.target) })),
  );
  const [runTargets, setRunTargets] = useState<DestinationTarget[] | null>(null);
  const [rows, setRows] = useState<Record<string, TransferRow>>({});
  const [phase, setPhase] = useState<TransferPhase>("idle");
  const archive = useRef<Blob | null>(null);
  const cancelRequested = useRef(false);
  const mounted = useRef(true);
  const transfer = useRef(0);
  const targets = runTargets ?? openTargets;
  const { setSelected, ...selection } = useTransferSelection(targets);
  const exporting = phase === "exporting";

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      cancelRequested.current = true;
      archive.current = null;
    };
  }, []);

  const live = (run: number) => mounted.current && transfer.current === run;

  const setRow = (id: string, row: TransferRow, run: number) => {
    if (live(run)) setRows((current) => ({ ...current, [id]: row }));
  };

  const sendTo = (target: DestinationTarget, ref: string, retryRef: string, run: number) =>
    transferToTarget({
      api: target.api,
      archive: () => archive.current,
      ref,
      retryRef,
      report: (row) => setRow(target.id, row, run),
      current: () => transfer.current === run,
    });

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

  const exportArchive = async (selectedTargets: DestinationTarget[], run: number) => {
    try {
      archive.current = await source.images.downloadImageArchive(imageRef);
      return live(run);
    } catch (error) {
      if (live(run)) {
        setRows(rowsFor(selectedTargets, { status: "failed", error: errorText(error) }));
        setPhase("idle");
      }
      return false;
    }
  };

  const start = async () => {
    const selectedTargets = targets.filter((target) => selection.selected.has(target.id));
    if (selectedTargets.length === 0) return;
    const run = ++transfer.current;
    cancelRequested.current = false;
    setRunTargets(targets);
    setPhase("exporting");
    setRows(rowsFor(selectedTargets, { status: "exporting" }));
    if (!(await exportArchive(selectedTargets, run))) return;

    if (live(run)) {
      setRows(rowsFor(selectedTargets, { status: "queued" }));
      setPhase("transferring");
    }

    for (let index = 0; index < selectedTargets.length; index += 1) {
      const target = selectedTargets[index];
      if (transfer.current !== run) return;
      if (cancelRequested.current) {
        for (const remaining of selectedTargets.slice(index))
          setRow(remaining.id, { status: "cancelled" }, run);
        break;
      }
      if ((await sendTo(target, imageRef, imageRef, run)) === "stale") return;
    }
    if (live(run)) {
      setPhase("idle");
      if (!cancelRequested.current) onComplete();
    }
  };

  const retry = async (targetID: string) => {
    const target = runTargets?.find((candidate) => candidate.id === targetID);
    const retryRef = rows[targetID]?.retryRef;
    const normalizedRetryRef = retryRef?.trim() ?? "";
    if (!target || !retryRef || !normalizedRetryRef || !archive.current) return;
    const run = ++transfer.current;
    setPhase("transferring");
    const outcome = await sendTo(target, normalizedRetryRef, retryRef, run);
    if (outcome === "transferred" && live(run)) onComplete();
    if (live(run)) setPhase("idle");
  };

  const editRetryRef = (id: string, retryRef: string) =>
    setRows((current) => ({ ...current, [id]: { ...current[id], retryRef } }));

  return {
    targets,
    rows,
    phase,
    ...selection,
    start,
    retry,
    cancel,
    close,
    editRetryRef,
  };
}
