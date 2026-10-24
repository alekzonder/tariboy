import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import {
  listDaemons,
  getActiveId,
  peekActiveId,
  setActiveId,
  resolveDaemon,
  unresolvedDaemon,
  type Daemon,
  type DaemonMeta,
} from "@/lib/daemons";
import { getActiveDaemon, setActiveDaemon } from "@/lib/api";
import { ApiProvider } from "./ApiProvider";
import { DaemonContext } from "./daemonContext";
import { daemonState, onHostState } from "@/lib/desktop";

// DaemonProvider owns the registry state and is the single place that pushes the
// active daemon into the API layer (setActiveDaemon). It also mounts the
// ApiProvider for the active host, so components outside a narrower server
// boundary address the selected host. On mount it re-applies the persisted
// active id (null = same-origin).
export function DaemonProvider({ children }: { children: ReactNode }) {
  const [daemons, setDaemons] = useState<DaemonMeta[]>([]);
  const [appVersion, setAppVersion] = useState("");
  const [activeId, setActiveIdState] = useState(() => {
    const id = peekActiveId();
    setActiveDaemon(id ? unresolvedDaemon(id) : null);
    return id;
  });
  const [active, setActiveState] = useState<Daemon | null>(getActiveDaemon);
  const applyActive = useCallback((next: Daemon | null) => {
    setActiveDaemon(next);
    setActiveState(next);
  }, []);
  const [registryError, setRegistryError] = useState("");
  // Every explicit selection invalidates older async hydration/refresh work.
  // Without this guard, the provider's mount fetch can finish after an agent
  // workspace selects its route host and silently restore the previously
  // persisted daemon underneath the newly rendered tab.
  const selectionRevision = useRef(0);
  // Persisted selections are serialized too. On desktop, setActiveId awaits
  // migration before writing localStorage; a second selection must not be
  // overtaken by that delayed write.
  const selectionQueue = useRef<Promise<void>>(Promise.resolve());

  const refresh = useCallback(async () => {
    await selectionQueue.current;
    const revision = selectionRevision.current;
    try {
      const [next, selected] = await Promise.all([listDaemons(), getActiveId()]);
      if (revision !== selectionRevision.current) return;
      let active;
      let error = "";
      try {
        active = selected ? await resolveDaemon(selected) : null;
        if (selected && !active) {
          active = unresolvedDaemon(selected, next.find((host) => host.id === selected)?.label);
        }
        // A registered SSH host can transiently lose its tunnel endpoint.
        // Keep the route's availability state local so its shell does not get
        // replaced by a global registry failure.
      } catch (cause) {
        const label = next.find((host) => host.id === selected)?.label;
        active = unresolvedDaemon(selected, label);
        if (!next.some((host) => host.id === selected)) error = String(cause);
      }
      if (revision !== selectionRevision.current) return;
      setDaemons(next);
      setActiveIdState(selected);
      applyActive(active);
      setRegistryError(error);
    } catch (error) {
      if (revision === selectionRevision.current) setRegistryError(String(error));
    }
  }, [applyActive]);

  const select = useCallback(
    async (id: string) => {
      const revision = ++selectionRevision.current;
      const operation = selectionQueue.current.then(async (): Promise<boolean> => {
        try {
          if (revision !== selectionRevision.current) return false;
          const active = id ? await resolveDaemon(id) : null;
          if (revision !== selectionRevision.current) return false;
          if (id && !active) throw new Error(`host ${id} was not found`);
          await setActiveId(id);
          if (revision !== selectionRevision.current) return false;
          setActiveIdState(id);
          applyActive(active);
          setRegistryError("");
          return true;
        } catch (error) {
          if (revision === selectionRevision.current) setRegistryError(String(error));
          return false;
        }
      });
      selectionQueue.current = operation.then(
        () => undefined,
        () => undefined,
      );
      return operation;
    },
    [applyActive],
  );

  useEffect(() => {
    let cancelled = false;
    const revision = selectionRevision.current;
    void (async () => {
      try {
        const [next, selected] = await Promise.all([listDaemons(), getActiveId()]);
        let active;
        let error = "";
        try {
          active = selected ? await resolveDaemon(selected) : null;
          if (selected && !active) {
            active = unresolvedDaemon(selected, next.find((host) => host.id === selected)?.label);
          }
          // See refresh(): known hosts without a current endpoint are a
          // route-local unavailable state, not a registry failure.
        } catch (cause) {
          active = unresolvedDaemon(selected, next.find((host) => host.id === selected)?.label);
          if (!next.some((host) => host.id === selected)) error = String(cause);
        }
        if (cancelled || revision !== selectionRevision.current) return;
        setDaemons(next);
        setActiveIdState(selected);
        applyActive(active);
        setRegistryError(error);
      } catch (error) {
        if (!cancelled) setRegistryError(String(error));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [applyActive]);

  useEffect(() => onHostState(() => void refresh()), [refresh]);

  useEffect(() => {
    let cancelled = false;
    void daemonState()
      .then((next) => {
        if (!cancelled) setAppVersion(next?.app_version ?? "");
      })
      .catch(() => {
        if (!cancelled) setAppVersion("");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <DaemonContext.Provider value={{ daemons, appVersion, activeId, select, refresh }}>
      {registryError && (
        <div
          role="alert"
          className="border-b border-destructive/40 bg-destructive/10 px-3 py-2 text-sm"
        >
          Host registry: {registryError}
        </div>
      )}
      <ApiProvider target={active}>{children}</ApiProvider>
    </DaemonContext.Provider>
  );
}
