import { createContext } from "react";

export const SettingsFilterContext = createContext<ReadonlySet<string> | null>(null);
