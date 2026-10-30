import { useState } from "react";
import { useDaemons } from "@/components/daemonContext";
import { HostStatus } from "@/components/HostStatus";
import { hostUpdateAvailable, removeDaemon, type DaemonMeta } from "@/lib/daemons";
import { hostConnect, hostUpdate } from "@/lib/desktop";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { useWorkspaces } from "@/lib/workspaces";
import { useHostsContext } from "@/pages/settings/hostsContext";
import { WorkspaceSwitcher } from "@/pages/terminals/HostWorkspaces";
import { ServerDialog } from "@/pages/terminals/ServerDialog";
import { Button } from "@/components/ui/button";

/** Settings → Hosts: every registered host, adding, updating and removing
 *  them, and the host workspaces. */
export default function DaemonsPage() {
  const { daemons, appVersion, refresh } = useDaemons();
  const [dialog, setDialog] = useState<DaemonMeta | "add" | null>(null);
  const [error, setError] = useState("");
  const behind = daemons.filter((host) => hostUpdateAvailable(host, appVersion));

  async function remove(host: DaemonMeta) {
    if (
      !window.confirm(
        `Remove host ${host.label} from this app? The host and its data stay as they are.`,
      )
    )
      return;
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

  // Each host behind the app version, updated in turn by the same native
  // command as its own Update button. One failure is reported and the
  // remaining hosts still go.
  async function updateAll() {
    const labels = behind.map((host) => host.label).join(", ");
    if (!window.confirm(`Update ${behind.length} host(s) to ${appVersion}? ${labels}`)) return;
    setError("");
    const failures: string[] = [];
    for (const host of behind) {
      try {
        await hostUpdate(host.id);
      } catch (cause) {
        failures.push(`${host.label}: ${String(cause)}`);
      }
    }
    if (failures.length > 0) setError(`Could not update ${failures.join("; ")}`);
    await refresh();
  }

  return (
    <div className="space-y-4 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold">Hosts</h1>
          <p className="text-sm text-muted-foreground">
            One place for local and remote agent sessions.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {behind.length > 0 && (
            <Button variant="outline" onClick={() => void updateAll()}>
              Update all ({behind.length})
            </Button>
          )}
          <Button onClick={() => setDialog("add")} data-tour={TOUR_ANCHORS.addHost}>
            Add host
          </Button>
        </div>
      </div>
      <HostWorkspacesBar />

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="space-y-2">
        {daemons.map((host) => (
          <div
            key={host.id}
            className="flex items-start justify-between gap-3 rounded border px-3 py-2"
          >
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

/** The host workspaces: a selector only when two or more exist, and the
 *  manager that creates, renames and fills them. */
function HostWorkspacesBar() {
  const context = useHostsContext();
  const workspaces = useWorkspaces();
  if (!context) return null;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-sm text-muted-foreground">Host workspaces</span>
      {workspaces.workspaces.length > 1 && (
        <WorkspaceSwitcher
          hosts={context.workspaceHosts}
          onSelect={() => undefined}
          onManage={context.onManageWorkspaces}
        />
      )}
      <Button variant="outline" size="sm" onClick={() => context.onManageWorkspaces(false)}>
        Manage workspaces…
      </Button>
    </div>
  );
}
