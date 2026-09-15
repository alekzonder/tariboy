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

export const CHAT_TYPES_KEY = "terminals:chat-types:v1";

/** The operator's chosen types, or the default set. A stored value is only
 *  honoured when it is a non-empty list of strings; anything else — including a
 *  blocked or cleared Web Storage — falls back to the default. */
export function loadChatTypes(): string[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(CHAT_TYPES_KEY) ?? "null");
    if (Array.isArray(parsed) && parsed.length > 0 && parsed.every((v) => typeof v === "string")) {
      return parsed as string[];
    }
  } catch {
    // Unusable UI preference must never keep the conversation from rendering.
  }
  return [...DEFAULT_CHAT_TYPES];
}

/** Persist the chosen types; saving the default set clears the preference so a
 *  later change to the default is picked up. */
export function saveChatTypes(types: string[]): void {
  try {
    const isDefault =
      types.length === DEFAULT_CHAT_TYPES.length
      && DEFAULT_CHAT_TYPES.every((type) => types.includes(type));
    if (isDefault) localStorage.removeItem(CHAT_TYPES_KEY);
    else localStorage.setItem(CHAT_TYPES_KEY, JSON.stringify(types));
  } catch {
    // A rejected write only costs the preference, not the view.
  }
}
