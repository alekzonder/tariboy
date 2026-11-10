import { createContext } from "react";

/** The titlebar elements that route pages portal their own controls into, so a
 *  control can live in the app chrome while its data stays with the page. */
export interface TitlebarSlots {
  /** After the product name: the workspace switcher. */
  workspace: HTMLElement | null;
  /** Before the Help menu: the global search. */
  search: HTMLElement | null;
}

export const TitlebarSlotContext = createContext<TitlebarSlots>({ workspace: null, search: null });
