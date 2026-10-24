import { readKey, removeKey, storageKeys, writeKey } from "@/lib/storage";

/** The message types a chat shows unless the operator picks another set. It
 *  mirrors the daemon's own default (internal/bus.DefaultChatTypes) so the two
 *  agree about what counts as conversation rather than as an agent's own wake. */
export const DEFAULT_CHAT_TYPES = [
  "task.question",
  "task.answered",
  "task.assigned",
  "task.triage",
  "message",
  "note",
  "group.request",
  "chat.*",
] as const;

/** Types the filter offers beyond the default set: an agent's own wakes, which
 *  are readable here on request but never part of the conversation by default. */
export const EXTRA_CHAT_TYPES = ["task.goal", "script.result", "schedule.*"] as const;

export const CHAT_TYPES_KEY = storageKeys.chatTypes.name;

/** The operator's chosen types, or the default set. A stored value is only
 *  honoured when it is a non-empty list of strings; anything else — including a
 *  blocked or cleared Web Storage — falls back to the default. */
export function loadChatTypes(): string[] {
  const parsed = readKey(storageKeys.chatTypes);
  if (Array.isArray(parsed) && parsed.length > 0 && parsed.every((v) => typeof v === "string")) {
    return parsed as string[];
  }
  return [...DEFAULT_CHAT_TYPES];
}

/** Persist the chosen types; saving the default set clears the preference so a
 *  later change to the default is picked up. A rejected write only costs the
 *  preference, not the view. */
export function saveChatTypes(types: string[]): void {
  const isDefault =
    types.length === DEFAULT_CHAT_TYPES.length &&
    DEFAULT_CHAT_TYPES.every((type) => types.includes(type));
  if (isDefault) removeKey(storageKeys.chatTypes);
  else writeKey(storageKeys.chatTypes, types);
}
