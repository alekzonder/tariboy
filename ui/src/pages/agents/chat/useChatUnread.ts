import { useCallback, useEffect, useState } from "react";
import { chatListOn, type ApiTarget } from "@/lib/api";
import { useMessagesSocket } from "@/hooks/useMessagesSocket";

/**
 * How many messages from one agent the customer has not read.
 *
 * It reads the same daemon-owned count the sidebar row reads, so the dot on the
 * Chat tab, the dot on the sidebar row and the chat's own toolbar cannot
 * disagree — unread lives in the conversation, not in the tab that renders it.
 * The tab is not mounted when another tab is open, which is exactly when the
 * dot has to be right, so this cannot live inside the chat itself.
 */
export function useChatUnread(target: ApiTarget, agent: string, enabled = true): number {
  const [unread, setUnread] = useState(0);
  const load = useCallback(async () => {
    if (!enabled || !agent) return;
    try {
      const page = await chatListOn(target);
      setUnread(page.chats?.find((chat) => chat.agent === agent)?.unread ?? 0);
    } catch {
      // A transient failure keeps the last known count rather than clearing it.
    }
    // target is derived from the host id and changes identity every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent, enabled, target?.baseURL]);

  // The socket only fires for new messages, and the chat's own Mark read and
  // Mark unread move this count without one; a slow poll keeps the dot honest
  // in those two cases without making the socket carry read state.
  useEffect(() => {
    void load();
    if (!enabled) return;
    const timer = window.setInterval(() => void load(), 10_000);
    return () => window.clearInterval(timer);
  }, [enabled, load]);
  useMessagesSocket({
    target,
    enabled: enabled && Boolean(agent) && (!target || Boolean(target.baseURL)),
    onHint: (hint) => { if (hint.agent === agent) void load(); },
    onOpen: () => { void load(); },
  });
  return unread;
}
