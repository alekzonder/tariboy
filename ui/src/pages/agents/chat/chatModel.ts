import type { ChatMessage, ChatSummary } from "@/lib/api";
import type { FeedItem } from "./chatFeed";
import { isKind } from "./chatKinds";

/** The shared chats one agent takes part in: chats no single agent owns, which
 *  is what a multi-agent conversation is. They sit beside the agent's own three
 *  because from its workspace they are another thread it is in. */
export function sharedChats(chats: ChatSummary[], agent: string): ChatSummary[] {
  return chats.filter(
    (chat) => !chat.agent && (chat.participants ?? []).includes(`agent:${agent}`),
  );
}

/** The chat id derived from the title: a channel segment, so lowercase letters,
 *  digits, '-' and '_' only. The daemon validates it again and owns the answer;
 *  this only spares the customer from typing one by hand. */
export function chatIdFromTitle(title: string): string {
  return title
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60);
}

/** The chat id the endpoints are addressed by, and whether the customer writes
 *  into it. An agent name still resolves to the personal chat, so the sidebar
 *  keeps working with what it knows. Only the conversation and the shared chats
 *  are written into: the other two are the agent's own notification feeds, and
 *  the customer is an observer in the service one. */
export function chatAddress({ name, selected }: { name: string; selected: string }) {
  const kind = isKind(selected);
  return {
    chatId: !name ? "" : kind ? `${selected}:${name}` : selected,
    writable: selected === "dm" || !kind,
  };
}

/** Only a participant's message can be unread, the rule the daemon counts by. */
export function chatParticipants({
  selected,
  customer,
  name,
  current,
}: {
  selected: string;
  customer: string;
  name: string;
  current: ChatSummary | undefined;
}): string[] {
  return isKind(selected) ? [customer, `agent:${name}`] : (current?.participants ?? []);
}

/** The feed narrowed by the search box: dividers stay, messages must match. */
export function filterFeed({ feed, query }: { feed: FeedItem[]; query: string }): FeedItem[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return feed;
  return feed.filter(
    (item) => item.kind !== "message" || item.message.text.toLowerCase().includes(needle),
  );
}

/** The message being published, shown with a `sent` receipt until the refetch
 *  brings back the stored one. */
export function pendingChatMessage({
  chatId,
  customer,
  text,
  now,
}: {
  chatId: string;
  customer: string;
  text: string;
  now: Date;
}): ChatMessage {
  return {
    id: `pending:${now.getTime()}`,
    channel: `chat:${chatId}`,
    ts: now.toISOString(),
    from: customer,
    source: customer,
    type: "message",
    text,
  };
}

/** A reply starts with the quoted message above whatever was already drafted. */
export function quoteReply({ text, draft }: { text: string; draft: string }): string {
  const quoted = text
    .split("\n")
    .map((line) => `> ${line}`)
    .join("\n");
  return `${quoted}\n\n${draft}`;
}
