import { useMemo, useRef, useState } from "react";
import type { HostAgents } from "@/lib/aggregate";
import { createApi } from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import { readKey, storageKeys, writeKey } from "@/lib/storage";
import { inWorkspace, useWorkspaces } from "@/lib/workspaces";
import type { ReorderKind } from "./sidebar/sidebarDnd";
import { mergeVisibleServerOrder, parseServerOrder, rankHosts } from "./terminalsHosts";

type SidebarOrder = NonNullable<HostAgents["sidebarOrder"]>;

/**
 * The sidebar's orders: the server order in browser storage, and each host's
 * group and agent order on that host. A host's saves run one at a time; the
 * newest edit shows at once and rolls back to the last saved order on failure.
 */
export function useSidebarReorder(
  sidebarHosts: HostAgents[],
  refresh: () => Promise<void>,
  setHostError: (error: string) => void,
) {
  const workspaces = useWorkspaces();
  const [serverOrder, setServerOrder] = useState<string[]>(() =>
    parseServerOrder(readKey(storageKeys.terminalsServerOrder)),
  );
  const [sidebarOrderOverrides, setSidebarOrderOverrides] = useState(
    () => new Map<string, SidebarOrder>(),
  );
  const sidebarSaveQueues = useRef(new Map<string, Promise<unknown>>());
  const sidebarSaveVersions = useRef(new Map<string, number>());
  const sidebarAcknowledgedOrders = useRef(new Map<string, SidebarOrder>());
  // Every host in server order; the sidebar shows the current workspace's.
  const rankedHosts = useMemo(
    () => rankHosts(sidebarHosts, serverOrder, sidebarOrderOverrides),
    [serverOrder, sidebarHosts, sidebarOrderOverrides],
  );
  const orderedSidebarHosts = useMemo(
    () => rankedHosts.filter((entry) => inWorkspace(workspaces, entry.host.id, workspaces.active)),
    [rankedHosts, workspaces],
  );

  const saveServerOrder = (visibleIds: string[]) => {
    const ids = mergeVisibleServerOrder(
      rankedHosts.map((entry) => entry.host.id),
      visibleIds,
    );
    const previous = serverOrder;
    setServerOrder(ids);
    if (!writeKey(storageKeys.terminalsServerOrder, { version: 1, ids })) {
      setServerOrder(previous);
      setHostError("Could not save server order: browser storage is unavailable or full");
    }
  };

  const setOverride = (id: string, order: SidebarOrder | undefined) =>
    setSidebarOrderOverrides((current) => {
      const copy = new Map(current);
      if (order) copy.set(id, order);
      else copy.delete(id);
      return copy;
    });

  const reorderSidebar = async (kind: ReorderKind, id: string, ids: string[]) => {
    setHostError("");
    if (kind === "servers") return saveServerOrder(ids);
    const host = orderedSidebarHosts.find((entry) => entry.host.id === id);
    if (!host) return;
    const previous = host.sidebarOrder ?? { version: 1, groups: [], agents: [] };
    const next = { ...previous, [kind]: ids };
    if (!sidebarSaveQueues.current.has(id)) sidebarAcknowledgedOrders.current.set(id, previous);
    const version = (sidebarSaveVersions.current.get(id) ?? 0) + 1;
    sidebarSaveVersions.current.set(id, version);
    setSidebarOrderOverrides((current) => new Map(current).set(id, next));
    const save = (sidebarSaveQueues.current.get(id) ?? Promise.resolve())
      .catch(() => undefined)
      .then(async () =>
        createApi(await resolveDaemon(id)).post("/api/daemon/config", {
          key: "sidebar_order_v1",
          value: JSON.stringify(next),
        }),
      );
    sidebarSaveQueues.current.set(id, save);
    try {
      await save;
      sidebarAcknowledgedOrders.current.set(id, next);
      if (sidebarSaveVersions.current.get(id) === version) {
        await refresh();
        setOverride(id, undefined);
      }
    } catch (cause) {
      if (sidebarSaveVersions.current.get(id) === version) {
        setOverride(id, sidebarAcknowledgedOrders.current.get(id) ?? previous);
        setHostError(`Could not save ${kind} order: ${String(cause)}`);
      }
    } finally {
      if (sidebarSaveQueues.current.get(id) === save) sidebarSaveQueues.current.delete(id);
    }
  };

  return { rankedHosts, orderedSidebarHosts, reorderSidebar };
}
