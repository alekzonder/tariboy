import { Button } from "@/components/ui/button";
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
import { hostConnected, isDesktop } from "@/lib/desktop";
import type { DaemonMeta } from "@/lib/daemons";
import { useServerForm } from "./server-dialog/useServerForm";
import { SshServerForm } from "./server-dialog/SshServerForm";
import { HttpsServerForm } from "./server-dialog/HttpsServerForm";

export interface ServerDialogProps {
  open: boolean;
  server?: DaemonMeta;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}

export function ServerDialog(props: ServerDialogProps) {
  const { open, server, onOpenChange } = props;
  const form = useServerForm(props);
  const {
    editing,
    label,
    setLabel,
    transport,
    setTransport,
    operation,
    formError,
    updateHost,
    submitSsh,
    dirtySsh,
    submitHttps,
  } = form;
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
          <div className="flex gap-2">
            <Button
              type="button"
              variant={transport === "ssh" ? "default" : "outline"}
              size="sm"
              onClick={() => setTransport("ssh")}
            >
              SSH
            </Button>
            <Button
              type="button"
              variant={transport === "https" ? "default" : "outline"}
              size="sm"
              onClick={() => setTransport("https")}
            >
              Advanced HTTPS
            </Button>
          </div>
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

          {transport === "ssh" ? (
            <SshServerForm {...form} />
          ) : (
            <HttpsServerForm {...form} />
          )}
          {formError && <p className="text-sm text-destructive">{formError}</p>}
        </div>

        <DialogFooter>
          {transport === "ssh" ? (
            <>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Close
              </Button>
              {operation.status === "running" ? (
                <Button type="button" disabled>
                  {operation.kind === "update" ? "Updating…" : "Connecting…"}
                </Button>
              ) : operation.status === "failed" && operation.kind === "update" ? (
                <Button type="button" onClick={() => void updateHost()}>
                  Retry update
                </Button>
              ) : operation.status === "failed" && operation.kind === "connect" ? (
                <>
                  <Button type="button" variant="outline" onClick={() => void submitSsh("connect")}>
                    Retry connection
                  </Button>
                  <Button type="button" onClick={() => void submitSsh("install")}>
                    Install Tariboy
                  </Button>
                </>
              ) : !editing ? (
                <Button type="button" onClick={() => void submitSsh()}>
                  Add and connect
                </Button>
              ) : dirtySsh || !hostConnected(server?.state) ? (
                <Button type="button" onClick={() => void submitSsh()}>
                  Save and reconnect
                </Button>
              ) : (
                <Button type="button" onClick={() => void updateHost()}>
                  Update Tariboy
                </Button>
              )}
            </>
          ) : (
            <Button type="button" onClick={() => void submitHttps()}>
              {editing ? "Save" : "Add"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
