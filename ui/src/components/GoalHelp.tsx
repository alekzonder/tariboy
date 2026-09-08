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
            Tariboy chooses an assigned Native Task without a pull request by
            priority, then prefers in-progress over open tasks, earlier creation
            time, and finally task key. Tariboy keeps the selected task until it
            is released by completion, cancellation, a pull request, reassignment,
            deletion or loss of access, disabling Goal, or a customer wait that
            exceeds the configured timeout.
          </DialogDescription>
        </DialogHeader>
      </DialogContent>
    </Dialog>
  );
}
