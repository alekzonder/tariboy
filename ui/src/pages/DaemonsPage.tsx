import { useState } from "react";
import { useDaemons } from "@/components/DaemonProvider";
import { HostStatus } from "@/components/HostStatus";
import { removeDaemon, type DaemonMeta } from "@/lib/daemons";
import { hostConnect } from "@/lib/desktop";
import { ServerDialog } from "@/pages/terminals/ServerDialog";
import { Button } from "@/components/ui/button";

export default function DaemonsPage() {
  const { daemons, appVersion, refresh } = useDaemons();
  const [dialog, setDialog] = useState<DaemonMeta | "add" | null>(null);
  const [error, setError] = useState("");

  async function remove(host: DaemonMeta) {
    if (!window.confirm(
      `Remove host ${host.label} from this app? The remote daemon and data remain on the server.`,
    )) return;
    try {
      await removeDaemon(host.id);
      await refresh();
    } catch (cause) {
      setError(String(cause));
    }
  }

  async function connect(host: DaemonMeta) {
    setError("");
    try {
      await hostConnect(host.id);
      await refresh();
    } catch (cause) {
      setError(`Could not connect to ${host.label}: ${String(cause)}`);
    }
  }

  return (
    <div className="space-y-4 p-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold">Hosts</h1>
          <p className="text-sm text-muted-foreground">
            One place for local and remote agent sessions.
          </p>
        </div>
        <Button onClick={() => setDialog("add")}>Add host</Button>
      </div>

      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <div className="space-y-2">
        {daemons.map((host) => (
          <div key={host.id} className="flex items-start justify-between gap-3 rounded border px-3 py-2">
            <div className="min-w-0 space-y-1">
              <div className="font-medium">{host.label}</div>
              <div className="truncate text-xs text-muted-foreground">
                {host.kind === "ssh" ? host.sshAlias : host.baseURL}
              </div>
              <HostStatus
                host={host}
                appVersion={appVersion}
                onConnect={() => void connect(host)}
                onUpdate={() => setDialog(host)}
              />
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => setDialog(host)}>
                Edit
              </Button>
              <Button variant="secondary" size="sm" onClick={() => void remove(host)}>
                Remove
              </Button>
            </div>
          </div>
        ))}
        {daemons.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No remote hosts yet. Add an SSH alias to get started.
          </p>
        )}
      </div>

      <ServerDialog
        open={dialog !== null}
        server={dialog && dialog !== "add" ? dialog : undefined}
        onOpenChange={(next) => {
          if (!next) setDialog(null);
        }}
        onSaved={() => void refresh()}
      />
    </div>
  );
}
