import { useEffect, useState, type ReactNode } from "react";
import { useDaemons } from "@/components/DaemonProvider";

export function RouteHostBoundary({ hostId, unavailable = false, children }: {
  hostId: string;
  unavailable?: boolean;
  children: ReactNode;
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
    !!host?.baseURL && (host.kind !== "ssh" || host.state === "ready")
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
  if (!hostReady || unavailable) {
    return (
      <>
        <p role="status" className="mb-2 text-sm text-muted-foreground">
          This host is reconnecting; actions are temporarily unavailable.
        </p>
        <div inert={true} aria-disabled="true" className="opacity-60">
          {children}
        </div>
      </>
    );
  }
  return children;
}
