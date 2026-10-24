import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useApi } from "@/components/apiContext";
import { guard } from "@/lib/toast-guard";
import { ActionRow, DraftField } from "./ConfigControls";
import { fieldOfKey, type ConfigField } from "./registry";
import type { BlockProps } from "./ConfigurationBlocks";
import { errorText } from "@/lib/errors";

const field = (key: string) => fieldOfKey(key) as ConfigField;

export function RetentionBlock({ draft, matched, sources, name }: BlockProps) {
  const api = useApi();
  const policy = sources.retention.value;
  // Prune is the one destructive action on this page: it is confirmed first,
  // it cannot be submitted twice while in flight, and a failure is reported
  // in place so the operator can read the reason and retry.
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pruning, setPruning] = useState(false);
  const [pruneError, setPruneError] = useState("");
  const prune = async () => {
    setPruning(true);
    setPruneError("");
    try {
      await api.agentPost(name, "prune", { "dry-run": false });
      await sources.reloadAll();
    } catch (e) {
      setPruneError(errorText(e));
    }
    setPruning(false);
  };
  const inherits = (def: number | undefined) =>
    def === undefined ? "" : `0 = server default (${def}).`;
  return (
    <>
      {sources.retention.error && (
        <p role="alert" className="text-sm text-destructive">
          Retention policy unavailable: {sources.retention.error}
        </p>
      )}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <DraftField
          field={{
            ...field("keep-iterations"),
            helper: inherits(policy?.default?.keep_iterations),
          }}
          draft={draft}
          matched={matched.has("keep-iterations")}
        />
        <DraftField
          field={{ ...field("keep-days"), helper: inherits(policy?.default?.keep_days) }}
          draft={draft}
          matched={matched.has("keep-days")}
        />
      </div>
      {policy && (
        <p className="text-xs text-muted-foreground">
          Effective: {policy.keep_iterations} iterations · {policy.keep_days} days · max_bytes{" "}
          {policy.max_bytes} (0 = unlimited). Pruned iterations are archived with their database
          rows first.
        </p>
      )}
      <ActionRow>
        <Button
          size="sm"
          variant="outline"
          onClick={() =>
            void guard("prune (dry-run)", () => api.agentPost(name, "prune", { "dry-run": true }), {
              verb: "ok",
            })
          }
        >
          Preview cleanup
        </Button>
        <Button
          size="sm"
          variant="destructive"
          disabled={pruning}
          onClick={() => setConfirmOpen(true)}
        >
          Prune now
        </Button>
      </ActionRow>
      {pruneError && (
        <p role="alert" className="text-xs text-destructive">
          {pruneError}
        </p>
      )}
      {/* Closed means unmounted, so no stray Cancel/confirm button sits in
          the page waiting to be matched by a broader query. */}
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Prune retained data?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes data selected by the current retention policy. Review the
              preview first if you need to check what will be removed.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                setConfirmOpen(false);
                void prune();
              }}
            >
              Prune retained data
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
