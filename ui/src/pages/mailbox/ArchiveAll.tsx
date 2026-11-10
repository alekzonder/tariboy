import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

/** Archive all: closes every open letter of the customer's Inbox after a
 *  confirmation. The dialog stays open when the daemon refuses. It renders
 *  nothing unless `shown` and a letter is open. */
export function ArchiveAll({
  shown,
  count,
  busy,
  onConfirm,
}: {
  shown: boolean;
  count: number;
  busy: boolean;
  onConfirm: () => Promise<boolean>;
}) {
  const [open, setOpen] = useState(false);
  if (!shown || count === 0) return null;
  const confirm = async () => {
    if (await onConfirm()) setOpen(false);
  };
  return (
    <>
      <Button size="sm" variant="outline" onClick={() => setOpen(true)} disabled={busy}>
        Archive all
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!next && !busy) setOpen(false);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Archive all</DialogTitle>
            <DialogDescription>
              Archive all {count} open letters in your inbox? Agents get no reply to their requests.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)} disabled={busy}>
              Cancel
            </Button>
            <Button onClick={() => void confirm()} disabled={busy}>
              Archive all
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
