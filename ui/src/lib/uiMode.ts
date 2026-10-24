import { useSyncExternalStore } from "react";
import { readKey, storageKeys, writeKey } from "./storage";

/**
 * Simple shows an ordinary user only the agent's Chat, Tasks, Terminal and
 * Configuration; Expert shows everything. It belongs to this Desktop WebView,
 * not to a daemon, and anything but an explicit "expert" — including unreadable
 * storage — is Simple. It hides UI only: it is not an access control.
 */
export type UiMode = "simple" | "expert";

export const UI_MODE_KEY = storageKeys.uiMode.name;

const listeners = new Set<() => void>();

export function readUiMode(): UiMode {
  return readKey(storageKeys.uiMode);
}

/** The tab an agent opens on: Chat in Simple, Terminal in Expert. */
export function defaultAgentTab(): "chat" | "console" {
  return readUiMode() === "expert" ? "console" : "chat";
}

export function writeUiMode(mode: UiMode): void {
  // Unavailable storage leaves the interface in Simple rather than breaking it.
  writeKey(storageKeys.uiMode, mode);
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function useUiMode(): UiMode {
  return useSyncExternalStore(subscribe, readUiMode);
}
