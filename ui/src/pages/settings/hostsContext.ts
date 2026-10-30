import { useOutletContext } from "react-router-dom";
import type { WorkspaceHost } from "@/pages/terminals/HostWorkspaces";

/** What Settings → Hosts takes from the console: the host workspaces, whose
 *  manager dialog the console owns. */
export interface HostsOutletContext {
  workspaceHosts: WorkspaceHost[];
  /** Opens the workspace manager; `focusNew` for "New workspace…". */
  onManageWorkspaces: (focusNew: boolean) => void;
}

/** The console's host workspaces, or undefined outside the Settings route. */
export function useHostsContext() {
  return useOutletContext<HostsOutletContext | undefined>();
}
