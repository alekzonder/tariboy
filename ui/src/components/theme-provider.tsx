import { useEffect, useState } from "react";
import { readKey, storageKeys, writeKey } from "@/lib/storage";
import { ThemeProviderContext, type Theme, type ThemeProviderState } from "./themeContext";

type ThemeProviderProps = {
  children: React.ReactNode;
  defaultTheme?: Theme;
};

function systemTheme(): "light" | "dark" {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function applyTheme(theme: Theme) {
  const root = window.document.documentElement;
  root.classList.remove("light", "dark");
  root.classList.add(theme === "system" ? systemTheme() : theme);
}

export function ThemeProvider({ children, defaultTheme = "system" }: ThemeProviderProps) {
  const [theme, setThemeState] = useState<Theme>(() =>
    readKey({ ...storageKeys.theme, fallback: defaultTheme }),
  );

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  // Follow the OS preference live while on "system".
  useEffect(() => {
    if (theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = () => applyTheme("system");
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, [theme]);

  const value: ThemeProviderState = {
    theme,
    setTheme: (next: Theme) => {
      writeKey(storageKeys.theme, next);
      setThemeState(next);
    },
  };

  return <ThemeProviderContext.Provider value={value}>{children}</ThemeProviderContext.Provider>;
}
