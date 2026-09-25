import { createContext } from "react";

/** The titlebar element that route pages portal their own controls into, so a
 *  control can live in the app chrome while its data stays with the page. */
export const TitlebarSlotContext = createContext<HTMLElement | null>(null);
