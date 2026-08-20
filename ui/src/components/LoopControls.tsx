import { toast } from "sonner";
import { loopEnable, loopDisable, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

const err = (e: unknown) => toast.error(e instanceof ApiError ? e.message : String(e));

// ConfirmButton is a destructive/guarded action wrapped in an AlertDialog so the
// operator must confirm before the action fires (used for Restart / Kill / Pause).
export function ConfirmButton({
  label, description, variant = "outline", onConfirm,
}: {
  label: string;
  description: string;
  variant?: "outline" | "destructive";
  onConfirm: () => Promise<void>;
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild><Button size="sm" variant={variant}>{label}</Button></AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{label}?</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={() => { void onConfirm().catch(err); }}>{label}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// LoopToggle flips the persisted Autopilot flag (loop/enable|disable) without
// launching a run.
// Pausing is confirm-guarded; starting is a plain click.
export function LoopToggle({
  name, enabled, onChanged,
}: { name: string; enabled: boolean; onChanged: () => void }) {
  if (enabled) {
    return (
      <ConfirmButton
        label="Pause"
        description="Autopilot is paused — no scheduled iterations start until you enable it again."
        onConfirm={async () => { await loopDisable(name); toast.success("Autopilot paused"); onChanged(); }}
      />
    );
  }
  return (
    <Button
      size="sm"
      onClick={() => { loopEnable(name).then(() => { toast.success("loop enabled"); onChanged(); }, err); }}
    >
      Start
    </Button>
  );
}
