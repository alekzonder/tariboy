import { createContext, useContext } from "react";
import type { DaemonMeta } from "@/lib/daemons";

export interface DaemonCtx {
  daemons: DaemonMeta[];
  appVersion: string;
  activeId: string;
  select: (id: string) => Promise<boolean>;
  refresh: () => Promise<void>;
}

export const DaemonContext = createContext<DaemonCtx | null>(null);

export function useDaemons(): DaemonCtx {
  const v = useContext(DaemonContext);
  if (!v) throw new Error("useDaemons must be used within a DaemonProvider");
  return v;
}

// Detail components are also rendered in focused unit tests and embeddable
// shells. In the full app this returns the provider selection; without a
// provider it gives callers a stable local-host fallback.
export function useOptionalDaemons(): DaemonCtx | null {
  return useContext(DaemonContext);
}
