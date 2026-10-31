import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
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
import { useAgentStatus } from "@/lib/agent";
import { ApiError } from "@/lib/api";
import { mailKeys } from "@/pages/mailbox/mailQueries";

/** Clear inbox: deletes every open letter to the agent after a confirmation.
 *  Shared by the Mailbox toolbar and Settings → Advanced → Mail. */
export function ClearInbox({ name, disabled }: { name: string; disabled: boolean }) {
  const api = useApi();
  const client = useQueryClient();
  const { refresh } = useAgentStatus();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  const clear = async () => {
    setBusy(true);
    try {
      const result = await api.mail.clear(name);
      toast.success(`deleted ${result.deleted} letter${result.deleted === 1 ? "" : "s"}`);
      setOpen(false);
    } catch (error) {
      toast.error(`clear failed: ${error instanceof ApiError ? error.message : String(error)}`);
    } finally {
      await Promise.all([
        client.invalidateQueries({ queryKey: mailKeys.server(api.target) }),
        refresh(),
      ]);
      setBusy(false);
    }
  };

  return (
    <>
      <Button size="sm" variant="outline" onClick={() => setOpen(true)} disabled={disabled}>
        Clear inbox
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!next && !busy) setOpen(false);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Clear inbox</DialogTitle>
            <DialogDescription>
              Delete every open letter in {name}&apos;s inbox. The agent never reads them. This
              cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)} disabled={busy}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={() => void clear()} disabled={busy}>
              Clear inbox
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
