import { createContext, useContext } from "react";
import type { AgentStatus } from "@/lib/types";

export const AgentNameContext = createContext<string>("");
export const useAgentName = () => useContext(AgentNameContext);

// The live agent status polled once in AgentLayout (for the unified header) and
// shared down to the tabs so a tab like Overview reads the same snapshot instead
// of opening a second identical /status poll. `refresh` is the poll's own tick,
// so a tab can force an immediate refetch after an action (loop toggle, restart).
export interface AgentStatusCtx {
  status: AgentStatus | null;
  refresh: () => Promise<void>;
}

export const AgentStatusContext = createContext<AgentStatusCtx>({
  status: null,
  refresh: async () => {},
});
export const useAgentStatus = () => useContext(AgentStatusContext);
