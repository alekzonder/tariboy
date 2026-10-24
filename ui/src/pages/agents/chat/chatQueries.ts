import { queryOptions } from "@tanstack/react-query";
import type { ApiClient } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { apiServerKey } from "@/lib/queryClient";

// Keys are built from the client's host; credentials never enter a key.
type Target = Daemon | null;
export const chatKeys = {
  server: (target: Target) => [...apiServerKey(target), "chats"] as const,
  list: (target: Target) => [...chatKeys.server(target), "list"] as const,
  /** Every read of one conversation, whatever message types it filters by. */
  chat: (target: Target, chatId: string) =>
    [...chatKeys.server(target), "messages", chatId] as const,
  messages: (target: Target, chatId: string, types: string[]) =>
    [...chatKeys.chat(target, chatId), types] as const,
};

export const chatQueries = {
  list: (api: ApiClient) =>
    queryOptions({
      queryKey: chatKeys.list(api.target),
      queryFn: () => api.messages.chatList(),
    }),
  messages: (api: ApiClient, chatId: string, types: string[]) => {
    const chat = chatKeys.chat(api.target, chatId);
    return queryOptions({
      queryKey: chatKeys.messages(api.target, chatId, types),
      queryFn: () => api.messages.chatMessages(chatId, { types }),
      enabled: !!chatId,
      // A conversation left behind is read again when it is opened, so its
      // unread mark is the daemon's current one, never a cached page's.
      gcTime: 0,
      // A new type filter keeps the conversation shown while it loads; a switch
      // of agent, host or chat never shows the previous one.
      placeholderData: (previous, query) =>
        chat.every((part, index) => query?.queryKey[index] === part) ? previous : undefined,
    });
  },
};
