import { useNavigate } from "react-router-dom";
import { useDaemons } from "@/components/daemonContext";
import { listDaemons, removeDaemon, type DaemonMeta } from "@/lib/daemons";
import { hostConnect } from "@/lib/desktop";

export type ServerDialogState = { mode: "add" } | { mode: "edit"; server: DaemonMeta };

/** What the console does to registered servers: connect, edit, remove. */
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
  const { daemons, refresh: refreshDaemons } = useDaemons();
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

  const removeServer = async (id: string) => {
    const server = (await listDaemons()).find((m) => m.id === id);
    if (
      !window.confirm(
        `Remove host ${server?.label ?? id} from this app? The host and its data stay as they are.`,
      )
    )
      return;
    await removeDaemon(id);
    // Leave a route whose host no longer exists. targetFor() also fails closed,
    // but navigating away gives the user a useful screen instead of a dead pane.
    if (hostId === id) navigate("/");
    await registryChanged();
  };

  return { registryChanged, connectHost, editServer, removeServer };
}
