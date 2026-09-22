import type { ChatSummary } from "@/lib/api";

/** The shared chats one agent takes part in: chats no single agent owns, which
 *  is what a multi-agent conversation is. They sit beside the agent's own three
 *  because from its workspace they are another thread it is in. */
export function sharedChats(chats: ChatSummary[], agent: string): ChatSummary[] {
  return chats.filter((chat) => !chat.agent && (chat.participants ?? []).includes(`agent:${agent}`));
}

/** The chat id derived from the title: a channel segment, so lowercase letters,
 *  digits, '-' and '_' only. The daemon validates it again and owns the answer;
 *  this only spares the customer from typing one by hand. */
export function chatIdFromTitle(title: string): string {
  return title.trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "-")
    .replace(/^-+|-+$/g, "").slice(0, 60);
}
