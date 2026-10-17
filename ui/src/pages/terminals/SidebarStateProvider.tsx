import { useState, type PropsWithChildren } from "react";
import { COMPACT_QUERY, useMediaQuery } from "@/hooks/useMediaQuery";
import { SidebarStateContext } from "./sidebarStateContext";
import { useSidebarState } from "./useSidebarWidth";

export function SidebarStateProvider({ children }: PropsWithChildren) {
  const sidebar = useSidebarState();
  const compact = useMediaQuery(COMPACT_QUERY);
  // Not persisted: the drawer is a transient overlay, and the wide layout's
  // saved width and hidden state stay as they were.
  const [drawerOpen, setDrawerOpen] = useState(false);
  return (
    <SidebarStateContext.Provider value={{ ...sidebar, compact, drawerOpen: compact && drawerOpen, setDrawerOpen }}>
      {children}
    </SidebarStateContext.Provider>
  );
}
