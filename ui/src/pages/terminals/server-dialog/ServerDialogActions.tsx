import { Button } from "@/components/ui/button";
import { hostConnected } from "@/lib/desktop";
import type { DaemonMeta } from "@/lib/daemons";
import { sshFooterAction } from "./serverFormValidation";
import type { ServerForm } from "./useServerForm";

/** SSH or HTTPS first, offered only when adding a host in the desktop shell. */
export function TransportToggle({
  transport,
  setTransport,
}: Pick<ServerForm, "transport" | "setTransport">) {
  return (
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
  );
}

function SshPrimaryAction({
  server,
  editing,
  operation,
  dirtySsh,
  updateHost,
  submitSsh,
}: { server?: DaemonMeta } & Pick<
  ServerForm,
  "editing" | "operation" | "dirtySsh" | "updateHost" | "submitSsh"
>) {
  switch (sshFooterAction(operation, editing, dirtySsh, hostConnected(server?.state))) {
    case "running":
      return (
        <Button type="button" disabled>
          {operation.kind === "update" ? "Updating…" : "Connecting…"}
        </Button>
      );
    case "retry-update":
      return (
        <Button type="button" onClick={() => void updateHost()}>
          Retry update
        </Button>
      );
    case "retry-connect":
      return (
        <>
          <Button type="button" variant="outline" onClick={() => void submitSsh("connect")}>
            Retry connection
          </Button>
          <Button type="button" onClick={() => void submitSsh("install")}>
            Install Tariboy
          </Button>
        </>
      );
    case "add":
      return (
        <Button type="button" onClick={() => void submitSsh()}>
          Add and connect
        </Button>
      );
    case "reconnect":
      return (
        <Button type="button" onClick={() => void submitSsh()}>
          Save and reconnect
        </Button>
      );
    case "update":
      return (
        <Button type="button" onClick={() => void updateHost()}>
          Update Tariboy
        </Button>
      );
  }
}

/** The dialog footer: Close plus the SSH action that fits the operation, or
 *  the HTTPS save. */
export function ServerDialogActions({
  form,
  server,
  onClose,
}: {
  form: ServerForm;
  server?: DaemonMeta;
  onClose: () => void;
}) {
  if (form.transport !== "ssh") {
    return (
      <Button type="button" onClick={() => void form.submitHttps()}>
        {form.editing ? "Save" : "Add"}
      </Button>
    );
  }
  return (
    <>
      <Button type="button" variant="outline" onClick={onClose}>
        Close
      </Button>
      <SshPrimaryAction server={server} {...form} />
    </>
  );
}
