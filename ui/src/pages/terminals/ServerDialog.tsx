import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { isDesktop } from "@/lib/desktop";
import type { DaemonMeta } from "@/lib/daemons";
import { useServerForm } from "./server-dialog/useServerForm";
import { SshServerForm } from "./server-dialog/SshServerForm";
import { HttpsServerForm } from "./server-dialog/HttpsServerForm";
import { ServerDialogActions, TransportToggle } from "./server-dialog/ServerDialogActions";

export interface ServerDialogProps {
  open: boolean;
  server?: DaemonMeta;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}

export function ServerDialog(props: ServerDialogProps) {
  const { open, server, onOpenChange } = props;
  const form = useServerForm(props);
  const { editing, label, setLabel, transport, setTransport, formError } = form;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] min-w-0 overflow-x-hidden overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            <span>{editing ? "Edit host" : "Add host"}</span>
            {editing && label.trim() && (
              <span className="font-normal text-muted-foreground"> · {label.trim()}</span>
            )}
          </DialogTitle>
          <DialogDescription className="sr-only">
            Configure an SSH or HTTPS connection to a Tariboy host.
          </DialogDescription>
        </DialogHeader>

        {!editing && isDesktop() && (
          <TransportToggle transport={transport} setTransport={setTransport} />
        )}

        <div className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="add-host-label">{transport === "ssh" ? "Label" : "label"}</Label>
            <Input
              id="add-host-label"
              value={label}
              onChange={(event) => setLabel(event.target.value)}
              className="h-8"
            />
          </div>

          {transport === "ssh" ? <SshServerForm {...form} /> : <HttpsServerForm {...form} />}
          {formError && <p className="text-sm text-destructive">{formError}</p>}
        </div>

        <DialogFooter>
          <ServerDialogActions form={form} server={server} onClose={() => onOpenChange(false)} />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
