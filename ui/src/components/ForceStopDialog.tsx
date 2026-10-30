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

/** The confirmation in front of Force-stop current run (the kill action). */
export function ForceStopDialog({
  name,
  open,
  setOpen,
  onForceStop,
}: {
  name: string;
  open: boolean;
  setOpen: (open: boolean) => void;
  onForceStop: () => void;
}) {
  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Force-stop the current run of {name}?</AlertDialogTitle>
          <AlertDialogDescription>
            This kills the running session at once. The agent stays on and starts its next run as
            usual.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onForceStop}>
            Force-stop
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
