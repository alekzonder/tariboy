import { HelpCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

export function GoalHelp() {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label="How Goal is selected"
          title="How Goal is selected"
        >
          <HelpCircle aria-hidden="true" />
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>How Goal is selected</DialogTitle>
          <DialogDescription>
            Tariboy chooses an assigned Native Task by priority, then prefers
            in-progress over open tasks, earlier creation time, and finally task
            key. Tariboy keeps the selected task until it is released by
            completion, cancellation, reassignment, deletion or loss of access,
            disabling Goal, or a customer wait that exceeds the configured
            timeout. Strictly higher-priority eligible work can preempt it. A pull
            request URL does not affect Goal selection or release.
          </DialogDescription>
        </DialogHeader>
      </DialogContent>
    </Dialog>
  );
}
