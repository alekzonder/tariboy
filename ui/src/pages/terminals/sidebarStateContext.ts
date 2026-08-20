import { createContext, useContext } from "react";

export interface SidebarStateValue {
  width: number;
  hidden: boolean;
  setWidth: (px: number) => void;
  setHidden: (hidden: boolean) => void;
}

export const SidebarStateContext = createContext<SidebarStateValue | null>(null);

export function useSharedSidebarState(): SidebarStateValue {
  const value = useContext(SidebarStateContext);
  if (!value) {
    throw new Error("useSharedSidebarState must be used inside SidebarStateProvider");
  }
  return value;
}
