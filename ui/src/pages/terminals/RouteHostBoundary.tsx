import { useEffect, useState, type ReactNode } from "react";
import { ApiProvider } from "@/components/ApiProvider";
import { useDaemons } from "@/components/DaemonProvider";
import { getActiveDaemon } from "@/lib/api";
import { hostConnected } from "@/lib/desktop";
import type { Daemon } from "@/lib/daemons";

export function RouteHostBoundary({ hostId, unavailable = false, children }: {
  hostId: string;
  unavailable?: boolean;
  children: ReactNode | ((target: Daemon | null) => ReactNode);
}) {
  const { activeId, daemons, select } = useDaemons();
  const [selection, setSelection] = useState<{
    hostId: string;
    status: "connecting" | "ready" | "unavailable";
  }>(() => ({ hostId, status: "connecting" }));
  const status = selection.hostId === hostId ? selection.status : "connecting";
  const host = daemons.find((entry) => entry.id === hostId);
  const knownHost = hostId === "" || host !== undefined;
  const hostReady = hostId === "" || (
    !!host?.baseURL && (host.kind !== "ssh" || hostConnected(host.state))
  );

  useEffect(() => {
    let cancelled = false;
    void select(hostId).then((selected) => {
      if (!cancelled) {
        setSelection({ hostId, status: selected ? "ready" : "unavailable" });
      }
    });
    return () => { cancelled = true; };
  }, [hostId, select]);

  if (!knownHost || status === "unavailable") {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        Host unavailable.
      </div>
    );
  }
  if (status !== "ready" || activeId !== hostId) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        Connecting to host…
      </div>
    );
  }
  // One tree shape for ready and reconnecting: switching wrappers would
  // remount the page and drop its loaded data and unsent drafts.
  const reconnecting = !hostReady || unavailable;
  const target = getActiveDaemon();
  return (
    <div className="flex h-full min-h-0 flex-col">
      {reconnecting && (
        <p role="status" className="mb-2 text-sm text-muted-foreground">
          This host is reconnecting; actions are temporarily unavailable.
        </p>
      )}
      <div
        inert={reconnecting}
        aria-disabled={reconnecting || undefined}
        className={reconnecting ? "min-h-0 flex-1 opacity-60" : "min-h-0 flex-1"}
      >
        {/* The route host is the server of everything below. */}
        <ApiProvider target={target}>
          {typeof children === "function" ? children(target) : children}
        </ApiProvider>
      </div>
    </div>
  );
}
