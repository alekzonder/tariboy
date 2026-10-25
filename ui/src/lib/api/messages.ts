import type {
  ChatFeed,
  ChatList,
  ChatReadResult,
  ChatView,
  InboxClearResult,
  InboxList,
  InboxProcessedResult,
  InboxReplyResult,
  InboxRequeueResult,
  MessageSendResult,
} from "../api-types.generated";
import type { Transport } from "./client";

export type {
  ChannelList,
  ChannelMessage,
  ChannelRow,
  ChannelTail,
  ChannelWatch,
  ChannelWatchList,
  ChatMessage,
  ChatSummaryRow,
  ChatView,
  InboxMessage,
  SubscriptionChannel,
  SubscriptionChannelList,
} from "../api-types.generated";

// Backend status filter: pending → Queue, processed → Archive, dlq → DLQ.
export type InboxStatus = "pending" | "processed" | "dlq" | "all";

const chatPath = (chat: string) => `/api/chats/${encodeURIComponent(chat)}`;

export function messagesApi(t: Transport) {
  return {
    // List an agent's inbox for one sub-view. The backend returns rows newest-first.
    agentInboxList: (name: string, status: InboxStatus, limit?: number, before?: string) => {
      const q = new URLSearchParams({ status });
      if (limit) q.set("limit", String(limit));
      if (before) q.set("before", before);
      return t.agentGet<InboxList>(name, `inbox?${q.toString()}`);
    },
    agentInboxClear: (name: string) => t.agentPost<InboxClearResult>(name, "inbox/clear", {}),
    // Mark a pending row processed (operator ack). `result` is mandatory; the
    // backend prefixes it `operator:` so the audit trail distinguishes human acks.
    agentInboxProcessed: (name: string, id: string, result: string) =>
      t.agentPost<InboxProcessedResult>(name, `inbox/${encodeURIComponent(id)}/processed`, {
        result,
      }),
    // Reply to a row (operator). Publishes the reply and auto-processes the row.
    agentInboxReply: (name: string, id: string, text: string) =>
      t.agentPost<InboxReplyResult>(name, `inbox/${encodeURIComponent(id)}/reply`, { text }),
    // Requeue a DLQ'd row back to pending.
    agentInboxRequeue: (name: string, id: string) =>
      t.agentPost<InboxRequeueResult>(name, `inbox/${encodeURIComponent(id)}/requeue`, {}),
    chatList: (types?: string[]) =>
      t.get<ChatList>(
        `/api/chats${types?.length ? `?types=${encodeURIComponent(types.join(","))}` : ""}`,
      ),
    // `chat` is a chat id (`tasks:worker`), and an agent name still resolves to
    // that agent's conversation, which is what the sidebar knows.
    chatMessages: (
      chat: string,
      // before is a message timestamp, not an id: the feed merges two channels
      // whose ids embed their own channel name and therefore do not sort together.
      opts: { types?: string[]; limit?: number; before?: string } = {},
    ) => {
      const q = new URLSearchParams();
      if (opts.types?.length) q.set("types", opts.types.join(","));
      if (opts.limit) q.set("limit", String(opts.limit));
      if (opts.before) q.set("before", opts.before);
      const qs = q.toString();
      return t.get<ChatFeed>(`${chatPath(chat)}${qs ? `?${qs}` : ""}`);
    },
    // ts is the timestamp of the newest message actually shown, never "now", so
    // a message arriving mid-render cannot be marked read without being seen.
    // The daemon only ever moves the mark forward, with one deliberate
    // exception: `exact` sets it to ts even when that moves it back, which is
    // how a chat is marked unread.
    chatRead: (chat: string, ts: string, opts: { exact?: boolean } = {}) =>
      t.post<ChatReadResult>(`${chatPath(chat)}/read`, opts.exact ? { ts, exact: true } : { ts }),
    // A chat the customer creates: it belongs to no single agent and may hold
    // several. The daemon refuses an id in a namespace it provisions itself
    // (dm:, tasks:, service:, group:, telegram:, plugin:) and an id already in
    // use, so two conversations can never end up sharing one channel.
    chatCreate: (chat: { id: string; title?: string; participants?: string[] }) =>
      t.post<ChatView>("/api/chats", {
        id: chat.id,
        title: chat.title ?? "",
        participants: (chat.participants ?? []).join(","),
      }),
    // Membership is what carries delivery: an agent added here is subscribed to
    // the chat channel, and one removed keeps the deliveries it was already given.
    chatJoin: (chat: string, principal: string, role?: string) =>
      t.post<ChatView>(
        `${chatPath(chat)}/participants`,
        role ? { principal, role } : { principal },
      ),
    chatLeave: (chat: string, principal: string) =>
      t.delete<ChatView>(`${chatPath(chat)}/participants/${encodeURIComponent(principal)}`),
    // Publish as the customer. A message sent into a chat channel needs no
    // reply_to: the daemon routes an agent reply into the chat that owns the
    // original. reply_to stays available for a send outside a chat.
    messageSend: (message: { channel: string; type?: string; text: string; reply_to?: string }) =>
      t.post<MessageSendResult>("/api/messages", message),
  };
}
