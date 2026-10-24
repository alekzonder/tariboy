import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import type { useTuiDraft } from "@/lib/tui";

/** Compose modal: edits the draft and, on Send, writes it to the stream as-is
 * (no trailing Enter), then clears it. Cancel clears it too. */
export function ComposeDialog({
  open,
  onOpenChange,
  draft,
  send,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  draft: ReturnType<typeof useTuiDraft>;
  send: (text: string) => void;
}) {
  const sendModal = () => {
    const text = draft.text;
    if (!text) {
      onOpenChange(false);
      return;
    }
    send(text);
    draft.clear();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh]">
        <DialogHeader>
          <DialogTitle>Send text to the terminal</DialogTitle>
          <DialogDescription>
            The text is injected as-is, with no trailing Enter — submit it yourself in the terminal.
          </DialogDescription>
        </DialogHeader>
        <Textarea
          value={draft.text}
          onChange={(e) => draft.setText(e.target.value)}
          className="min-h-40 max-h-[60vh] overflow-auto font-mono text-sm"
          aria-label="Text to inject"
        />
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              draft.clear();
              onOpenChange(false);
            }}
          >
            Cancel
          </Button>
          <Button onClick={sendModal}>Send</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
