import { useEffect, useState } from "react";
import { toast } from "sonner";
import { agentGet, agentPost, ApiError } from "@/lib/api";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";

interface Limits { keep_iterations: number; keep_days: number; max_bytes: number }
// The effective limits, the agent's own override (0 inherits) and the server default.
interface Policy extends Limits { override: Limits; default: Limits }

const IMMEDIATE = "Takes effect immediately.";

const inherits = (def: number | undefined) =>
  def === undefined ? IMMEDIATE : `0 = server default (${def}). ${IMMEDIATE}`;

const reason = (e: unknown) => (e instanceof ApiError ? e.message : String(e));

async function guard(label: string, fn: () => Promise<unknown>, after?: () => void) {
  try { await fn(); toast.success(`${label} ok`); after?.(); }
  catch (e) { toast.error(`${label} failed: ${reason(e)}`); }
}

export function RetentionPanel({ name }: { name: string }) {
  const [pol, setPol] = useState<Policy | null>(null);
  const [keepIt, setKeepIt] = useState("");
  const [keepDays, setKeepDays] = useState("");
  // Prune is the one destructive action on this page: it is confirmed first,
  // it cannot be submitted twice while in flight, and a failure is reported
  // in place so the operator can read the reason and retry.
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pruning, setPruning] = useState(false);
  const [pruneError, setPruneError] = useState("");

  const load = () =>
    agentGet<Policy>(name, "retention").then((p) => { setPol(p); setKeepIt(String(p.override.keep_iterations)); setKeepDays(String(p.override.keep_days)); }).catch(() => setPol(null));
  useEffect(() => { if (name) void load(); }, [name]);

  const prune = async () => {
    setPruning(true);
    setPruneError("");
    try {
      await agentPost(name, "prune", { "dry-run": false });
      toast.success("prune ok");
      await load();
    } catch (e) {
      setPruneError(reason(e));
      toast.error(`prune failed: ${reason(e)}`);
    }
    setPruning(false);
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Retention and cleanup</CardTitle>
        <CardDescription>Keep the history you need, then review cleanup safely.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="keep-it">Keep iterations</Label>
            <Input id="keep-it" value={keepIt} onChange={(e) => setKeepIt(e.target.value)}
              className="h-9" aria-describedby="keep-it-help" />
            <p id="keep-it-help" className="text-xs text-muted-foreground">{inherits(pol?.default.keep_iterations)}</p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="keep-days">Keep days</Label>
            <Input id="keep-days" value={keepDays} onChange={(e) => setKeepDays(e.target.value)}
              className="h-9" aria-describedby="keep-days-help" />
            <p id="keep-days-help" className="text-xs text-muted-foreground">{inherits(pol?.default.keep_days)}</p>
          </div>
        </div>

        {/* Saving the policy and previewing what it would remove are both
            non-destructive, so they stay outside the bounded group below. */}
        <div className="space-y-1.5">
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={() => void guard("retention", () => agentPost(name, "retention", { "keep-iterations": Number(keepIt), "keep-days": Number(keepDays) }), () => void load())}>Save policy</Button>
            <Button size="sm" variant="secondary" onClick={() => void guard("prune (dry-run)", () => agentPost(name, "prune", { "dry-run": true }))}>Preview cleanup</Button>
          </div>
          <p className="text-xs text-muted-foreground">{IMMEDIATE}</p>
        </div>

        {/* Read-only policy metadata: not editable, so no timing string. */}
        {pol && (
          <div className="text-xs text-muted-foreground">
            effective: {pol.keep_iterations} iterations · {pol.keep_days} days · max_bytes {pol.max_bytes} (0 = unlimited).
            Pruned iterations are archived with their database rows first.
          </div>
        )}

        <section aria-labelledby="delete-retained" className="space-y-1.5 rounded-lg border border-destructive/50 p-3">
          <h3 id="delete-retained" className="text-sm font-medium">Delete retained data</h3>
          <Button size="sm" variant="destructive" disabled={pruning} onClick={() => setConfirmOpen(true)}>
            Prune now
          </Button>
          <p className="text-xs text-muted-foreground">{IMMEDIATE}</p>
          {pruneError && <p role="alert" className="text-xs text-destructive">{pruneError}</p>}
        </section>

        {/* Closed means unmounted, so no stray Cancel/confirm button sits in
            the page waiting to be matched by a broader query. */}
        <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Prune retained data?</AlertDialogTitle>
              <AlertDialogDescription>
                This permanently removes data selected by the current retention policy. Review the preview first if you need to check what will be removed.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={() => { setConfirmOpen(false); void prune(); }}>
                Prune retained data
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardContent>
    </Card>
  );
}
