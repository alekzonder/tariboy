import { createContext, useContext } from "react";
import type { HostNeeds } from "@/lib/needsYou";

/** What needs the user across the current workspace's hosts: one read that
 *  the sidebar badges, Home and Inbox share. */
export interface NeedsYouValue {
  hosts: HostNeeds[];
  /** False until every host answered or failed once. */
  loaded: boolean;
  /** Reads every host again, after an action changed what needs the user. */
  reload: () => void;
}

export const NeedsYouContext = createContext<NeedsYouValue>({
  hosts: [],
  loaded: false,
  reload: () => {},
});

export const useNeedsYou = () => useContext(NeedsYouContext);
