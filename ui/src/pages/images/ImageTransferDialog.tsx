import type { Daemon } from "@/lib/daemons";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { TransferProgressList } from "./transfer/TransferProgressList";
import { TransferSummary } from "./transfer/TransferSummary";
import { TransferTargetList } from "./transfer/TransferTargetList";
import { useImageTransfer } from "./transfer/useImageTransfer";

interface ImageTransferDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  imageRef: string;
  daemons: Daemon[];
  onComplete: () => void;
}

export function ImageTransferDialog({ open, ...props }: ImageTransferDialogProps) {
  if (!open) return null;
  return <ImageTransferDialogSession {...props} />;
}

function ImageTransferDialogSession(props: Omit<ImageTransferDialogProps, "open">) {
  const { onOpenChange, imageRef } = props;
  const transfer = useImageTransfer(props);
  const { targets, rows, phase, selectedCount, cancel, close } = transfer;
  const exporting = phase === "exporting";
  const controlsDisabled = phase !== "idle";

  return (
    <Dialog
      open
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          if (!exporting) close();
          return;
        }
        onOpenChange(true);
      }}
    >
      <DialogContent
        showCloseButton={!exporting}
        onEscapeKeyDown={(event) => {
          if (exporting) event.preventDefault();
        }}
        onPointerDownOutside={(event) => {
          if (exporting) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>Transfer image {imageRef}</DialogTitle>
          <DialogDescription>
            Select the ready servers that should receive this runnable image.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          {targets.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No ready servers are available for transfer.
            </p>
          ) : (
            <>
              <TransferTargetList
                targets={targets}
                selected={transfer.selected}
                allSelected={transfer.allSelected}
                disabled={controlsDisabled}
                onToggleAll={transfer.toggleAll}
                onToggle={transfer.toggleTarget}
              />
              <TransferProgressList
                targets={targets}
                rows={rows}
                disabled={controlsDisabled}
                onRetryRefChange={transfer.editRetryRef}
                onRetry={(id) => void transfer.retry(id)}
              />
            </>
          )}
          <TransferSummary
            phase={phase}
            rows={rows}
            targetCount={targets.length}
            selectedCount={selectedCount}
          />
        </div>
        <DialogFooter>
          {phase !== "idle" ? (
            <Button variant="outline" onClick={cancel} disabled={exporting}>
              Cancel transfer
            </Button>
          ) : (
            <Button variant="outline" onClick={close}>
              Cancel
            </Button>
          )}
          <Button
            disabled={selectedCount === 0 || controlsDisabled}
            onClick={() => void transfer.start()}
          >
            Start transfer
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
