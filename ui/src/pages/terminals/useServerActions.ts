import { useNavigate } from "react-router-dom";
import { useDaemons } from "@/components/daemonContext";
import { listDaemons, removeDaemon, type DaemonMeta } from "@/lib/daemons";
import { hostConnect, hostUpdate } from "@/lib/desktop";

export type ServerDialogState = { mode: "add" } | { mode: "edit"; server: DaemonMeta };

/** What the console does to registered servers: connect, edit, update, remove. */
export function useServerActions({
  hostId,
  refresh,
  setHostError,
  setServerDialog,
}: {
  hostId: string | undefined;
  refresh: () => Promise<void>;
  setHostError: (error: string) => void;
  setServerDialog: (state: ServerDialogState) => void;
}) {
  const navigate = useNavigate();
  // DaemonProvider caches the registry in React state and is the only thing
  // that pushes the active daemon into the api client, so registry edits made
  // here have to notify it too — otherwise the rest of the SPA keeps using a
  // stale (or removed) host as its target.
  const { daemons, appVersion, refresh: refreshDaemons } = useDaemons();
  const labelOf = (id: string) => daemons.find((host) => host.id === id)?.label ?? id;

  // Re-read after any registry mutation: refresh() runs one immediate
  // fetchAllAgents() tick, which resolves every daemon straight from storage,
  // so labels/reachability update without waiting up to 3s for the next poll.
  const registryChanged = async () => {
    void refresh();
    await refreshDaemons();
  };

  const connectHost = async (id: string) => {
    setHostError("");
    try {
      await hostConnect(id);
      await refreshDaemons();
    } catch (cause) {
      setHostError(`Could not connect to ${labelOf(id)}: ${String(cause)}`);
    }
  };

  const editServer = async (id: string) => {
    const server = (await listDaemons()).find((m) => m.id === id);
    if (server) setServerDialog({ mode: "edit", server });
  };

  // Every host behind the app version, updated in turn by the same native
  // command the server dialog's Update button runs. One failure is reported
  // and the remaining hosts still go.
  const updateAllServers = async (ids: string[]) => {
    if (
      !window.confirm(
        `Update ${ids.length} server(s) to ${appVersion}? ${ids.map(labelOf).join(", ")}`,
      )
    )
      return;
    setHostError("");
    const failures: string[] = [];
    for (const id of ids) {
      try {
        await hostUpdate(id);
      } catch (cause) {
        failures.push(`${labelOf(id)}: ${String(cause)}`);
      }
    }
    if (failures.length > 0) setHostError(`Could not update ${failures.join("; ")}`);
    await registryChanged();
  };

  const removeServer = async (id: string) => {
    const server = (await listDaemons()).find((m) => m.id === id);
    if (
      !window.confirm(
        `Remove host ${server?.label ?? id} from this app? The remote daemon and data remain on the server.`,
      )
    )
      return;
    await removeDaemon(id);
    // Leave a route whose host no longer exists. targetFor() also fails closed,
    // but navigating away gives the user a useful screen instead of a dead pane.
    if (hostId === id) navigate("/");
    await registryChanged();
  };

  return { registryChanged, connectHost, editServer, updateAllServers, removeServer };
}
