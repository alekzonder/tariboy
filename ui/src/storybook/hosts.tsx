import { useEffect, useState, type ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createQueryClient } from "@/lib/queryClient";
import { readKey, removeKey, storageKeys, writeKey } from "@/lib/storage";

// Components behind the decorators in decorators.tsx: a decorator needs hooks,
// so it renders one of these.

export const STORYBOOK_DAEMON_URL = "http://storybook.invalid";

export function ThemeHost({ dark, children }: { dark: boolean; children: ReactNode }) {
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);
  return <>{children}</>;
}

export function QueryHost({ children }: { children: ReactNode }) {
  const [client] = useState(createQueryClient);
  useEffect(() => () => client.clear(), [client]);
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

type Saved = { daemons: unknown; active: string };

const snapshot = (): Saved => ({
  daemons: readKey(storageKeys.daemons),
  active: readKey(storageKeys.activeDaemon),
});

function applyRegistry() {
  writeKey(storageKeys.daemons, [
    { id: "storybook", label: "Storybook (no daemon)", baseURL: STORYBOOK_DAEMON_URL },
  ]);
  writeKey(storageKeys.activeDaemon, "storybook");
}

function restoreRegistry(saved: Saved) {
  if (saved.daemons === null) removeKey(storageKeys.daemons);
  else writeKey(storageKeys.daemons, saved.daemons);
  if (saved.active === "") removeKey(storageKeys.activeDaemon);
  else writeKey(storageKeys.activeDaemon, saved.active);
}

export function DaemonHost({ children }: { children: ReactNode }) {
  // The registry is written during the first render: DaemonProvider reads the
  // active id when it mounts, which is before any effect of this component.
  const [saved] = useState(() => {
    const before = snapshot();
    applyRegistry();
    return before;
  });
  useEffect(() => {
    applyRegistry();
    return () => restoreRegistry(saved);
  }, [saved]);
  return <>{children}</>;
}
