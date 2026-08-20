import type { PropsWithChildren } from "react";
import { SidebarStateContext } from "./sidebarStateContext";
import { useSidebarState } from "./useSidebarWidth";

export function SidebarStateProvider({ children }: PropsWithChildren) {
  const sidebar = useSidebarState();
  return (
    <SidebarStateContext.Provider value={sidebar}>
      {children}
    </SidebarStateContext.Provider>
  );
}
