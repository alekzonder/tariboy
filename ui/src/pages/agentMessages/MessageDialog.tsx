import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { dialogCopy } from "./agentMessagesModel";
import type { MessageDialogState } from "./useMessageDialog";

// The Mark-processed / Reply / Clear-queue dialog. Clear takes no text and
// confirms with a destructive button.
export default function MessageDialog({ state }: { state: MessageDialogState }) {
  const { dialog, text, setText, busy, close, submit } = state;
  const copy = dialogCopy(dialog?.mode);
  return (
    <Dialog
      open={dialog !== null}
      onOpenChange={(o) => {
        if (!o && !busy) close();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{copy.title}</DialogTitle>
          <DialogDescription>{copy.description}</DialogDescription>
        </DialogHeader>
        {dialog?.mode !== "clear" && (
          <Textarea
            autoFocus
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder={copy.placeholder}
            rows={4}
          />
        )}
        <DialogFooter>
          <Button variant="outline" onClick={close} disabled={busy}>
            Cancel
          </Button>
          <Button variant={copy.variant} onClick={() => void submit()} disabled={busy}>
            {copy.title}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
