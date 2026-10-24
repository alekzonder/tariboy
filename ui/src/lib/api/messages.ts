import type { Transport } from "./client";

// ---- Per-agent inbox (Messages tab) ----
// One inbox row: the immutable message fields plus this agent's aggregated
// delivery state. The backend omits optional fields when empty, so most are
// optional here; `result`/`processed_at` are present only on archive rows.
export interface InboxItem {
  id: string;
  channel: string;
  ts: string;
  source: string;
  type: string;
  text: string;
  attempts: number;
  dlq: boolean;
  produced_by_agent?: string;
  produced_in_iteration?: string;
  kind?: string; // event | request | reply
  subject?: Record<string, unknown>;
  data?: Record<string, unknown>;
  correlation_id?: string;
  in_reply_to?: string;
  reply_to?: string;
  deadline?: string;
  delivered_at?: string;
  processed_at?: string;
  result?: string;
}

// Backend status filter: pending → Queue, processed → Archive, dlq → DLQ.
export type InboxStatus = "pending" | "processed" | "dlq" | "all";

// ---- Chats ----
// A chat is the two existing inboxes read as one conversation: what the
// customer sent into the agent's inbox, and what that agent sent into the
// customer's own channel. The daemon owns the projection; the UI only renders
// it, sends through the ordinary message endpoint, and marks it read.
export interface ChatSummary {
  /** The chat's own id, e.g. `dm:worker`, `tasks:worker`, `service:worker`. */
  id: string;
  kind: string;
  title: string;
  channel?: string;
  /** The agent the chat belongs to, empty for a chat no single agent owns. */
  agent: string;
  last_ts: string;
  last_from: string;
  last_type: string;
  last_text: string;
  unread: number;
  read_ts?: string;
  /** Every principal taking part, so a group chat is recognisable without a
   *  second request per chat. */
  participants?: string[];
}

export interface ChatMessage {
  id: string;
  channel: string;
  ts: string;
  from: string;
  source: string;
  type: string;
  text: string;
  subject?: Record<string, unknown>;
  data?: Record<string, unknown>;
  kind?: string;
  in_reply_to?: string;
  reply_to?: string;
}

/** What every chat write answers with: the chat the caller now has. */
export interface ChatDetail {
  chat: string;
  kind: string;
  title: string;
  channel: string;
  agent: string;
  participants: string[];
}

const chatPath = (chat: string) => `/api/chats/${encodeURIComponent(chat)}`;

export function messagesApi(t: Transport) {
  return {
    // List an agent's inbox for one sub-view. The backend returns rows newest-first.
    agentInboxList: (name: string, status: InboxStatus, limit?: number, before?: string) => {
      const q = new URLSearchParams({ status });
      if (limit) q.set("limit", String(limit));
      if (before) q.set("before", before);
      return t.agentGet<{ messages: InboxItem[]; count: number }>(name, `inbox?${q.toString()}`);
    },
    agentInboxClear: (name: string) =>
      t.agentPost<{ deleted_deliveries: number; deleted_messages: number }>(
        name,
        "inbox/clear",
        {},
      ),
    // Mark a pending row processed (operator ack). `result` is mandatory; the
    // backend prefixes it `operator:` so the audit trail distinguishes human acks.
    agentInboxProcessed: (name: string, id: string, result: string) =>
      t.agentPost<{ id: string; processed_at: string; result: string }>(
        name,
        `inbox/${encodeURIComponent(id)}/processed`,
        { result },
      ),
    // Reply to a row (operator). Publishes the reply and auto-processes the row.
    agentInboxReply: (name: string, id: string, text: string) =>
      t.agentPost<{
        id: string;
        channel: string;
        in_reply_to: string;
        correlation_id: string;
        replied: boolean;
      }>(name, `inbox/${encodeURIComponent(id)}/reply`, { text }),
    // Requeue a DLQ'd row back to pending.
    agentInboxRequeue: (name: string, id: string) =>
      t.agentPost<{ id: string; requeued: boolean }>(
        name,
        `inbox/${encodeURIComponent(id)}/requeue`,
        {},
      ),
    chatList: (types?: string[]) =>
      t.get<{ customer: string; chats: ChatSummary[]; count: number }>(
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
      return t.get<{
        customer: string;
        agent: string;
        chat: string;
        kind: string;
        title: string;
        messages: ChatMessage[];
        count: number;
        // The mark the chat was last read at, so a reader can tell what is new.
        read_ts?: string;
      }>(`${chatPath(chat)}${qs ? `?${qs}` : ""}`);
    },
    // ts is the timestamp of the newest message actually shown, never "now", so
    // a message arriving mid-render cannot be marked read without being seen.
    // The daemon only ever moves the mark forward, with one deliberate
    // exception: `exact` sets it to ts even when that moves it back, which is
    // how a chat is marked unread.
    chatRead: (chat: string, ts: string, opts: { exact?: boolean } = {}) =>
      t.post<{ agent: string; chat: string; read_ts: string }>(
        `${chatPath(chat)}/read`,
        opts.exact ? { ts, exact: true } : { ts },
      ),
    // A chat the customer creates: it belongs to no single agent and may hold
    // several. The daemon refuses an id in a namespace it provisions itself
    // (dm:, tasks:, service:, group:, telegram:, plugin:) and an id already in
    // use, so two conversations can never end up sharing one channel.
    chatCreate: (chat: { id: string; title?: string; participants?: string[] }) =>
      t.post<ChatDetail>("/api/chats", {
        id: chat.id,
        title: chat.title ?? "",
        participants: (chat.participants ?? []).join(","),
      }),
    // Membership is what carries delivery: an agent added here is subscribed to
    // the chat channel, and one removed keeps the deliveries it was already given.
    chatJoin: (chat: string, principal: string, role?: string) =>
      t.post<ChatDetail>(
        `${chatPath(chat)}/participants`,
        role ? { principal, role } : { principal },
      ),
    chatLeave: (chat: string, principal: string) =>
      t.delete<ChatDetail>(`${chatPath(chat)}/participants/${encodeURIComponent(principal)}`),
    // Publish as the customer. A message sent into a chat channel needs no
    // reply_to: the daemon routes an agent reply into the chat that owns the
    // original. reply_to stays available for a send outside a chat.
    messageSend: (message: { channel: string; type?: string; text: string; reply_to?: string }) =>
      t.post<{ id: string; channel: string; sent: boolean }>("/api/messages", message),
  };
}
export type MessagesApi = ReturnType<typeof messagesApi>;
