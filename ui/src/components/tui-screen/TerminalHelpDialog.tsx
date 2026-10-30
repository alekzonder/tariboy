import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const HELP_TEXT =
  "Terminal is for working with the agent by hand. For work it should carry out " +
  "on its own, give it a task in Work; it runs it when it is on. " +
  "This is a live terminal. Click it to focus — keystrokes go straight to the " +
  "session in real time. Use the buttons under Keys for Esc/Enter/Ctrl-C/arrows, or " +
  "the Compose button to draft a multi-line message and inject it (no trailing " +
  "Enter). Double-click selects a word natively; drag to select and copy. " +
  "Scrollback opens tmux's pane history; leave it with q or Exit scrollback.";

export function TerminalHelpDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Terminal help</DialogTitle>
          <DialogDescription>{HELP_TEXT}</DialogDescription>
        </DialogHeader>
      </DialogContent>
    </Dialog>
  );
}
