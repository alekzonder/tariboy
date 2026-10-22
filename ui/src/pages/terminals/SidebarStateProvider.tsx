import { useState, type PropsWithChildren } from "react";
import { useLocation } from "react-router-dom";
import { DRAWER_QUERY, useMediaQuery } from "@/hooks/useMediaQuery";
import { SidebarStateContext } from "./sidebarStateContext";
import { useSidebarState } from "./useSidebarWidth";

export function SidebarStateProvider({ children }: PropsWithChildren) {
  const sidebar = useSidebarState();
  const compact = useMediaQuery(DRAWER_QUERY);
  const { key } = useLocation();
  // Not persisted: the drawer is a transient overlay, and the wide layout's
  // saved width and hidden state stay as they were. It belongs to the history
  // entry it opened on, so Back or any other route change closes it.
  const [openOn, setOpenOn] = useState<string | null>(null);
  // A wide window drops it, so turning the tablet back does not reopen it.
  if (!compact && openOn !== null) setOpenOn(null);
  return (
    <SidebarStateContext.Provider value={{
      ...sidebar,
      compact,
      drawerOpen: compact && openOn === key,
      setDrawerOpen: (open) => setOpenOn(open ? key : null),
    }}>
      {children}
    </SidebarStateContext.Provider>
  );
}
