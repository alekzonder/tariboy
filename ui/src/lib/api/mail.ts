import type {
  MailClearResult,
  MailCloseResult,
  MailList,
  MailReadResult,
  MailReplyResult,
  MailRequeueResult,
  MailSendResult,
  MailThread,
  MailUnread,
} from "../api-types.generated";
import type { Transport } from "./client";
import { queryPath } from "./tasks";

export type { Letter, MailList, MailUnread } from "../api-types.generated";

export type MailBox = "inbox" | "sent" | "archive";

// Every mail call acts as `as`; without it the daemon acts as the customer.
export interface MailListQuery {
  box: MailBox;
  as?: string;
  limit?: number;
  before?: string;
}
export interface MailSendInput {
  to: string[];
  subject?: string;
  text: string;
  notice?: boolean;
  as?: string;
}
export interface MailReplyInput {
  text: string;
  ask?: boolean;
  as?: string;
}

const letterPath = (id: string, rest = "") => `/api/mail/${encodeURIComponent(id)}${rest}`;

export function mailApi(t: Transport) {
  return {
    list: (query: MailListQuery, signal?: AbortSignal) =>
      t.request<MailList>("GET", queryPath("/api/mail", query), undefined, signal),
    show: (id: string, as?: string, signal?: AbortSignal) =>
      t.request<MailThread>("GET", queryPath(letterPath(id), { as }), undefined, signal),
    send: (input: MailSendInput) => t.post<MailSendResult>("/api/mail", input),
    reply: (id: string, input: MailReplyInput) =>
      t.post<MailReplyResult>(letterPath(id, "/reply"), input),
    close: (id: string, input: { note?: string; as?: string } = {}) =>
      t.post<MailCloseResult>(letterPath(id, "/close"), input),
    read: (ids: string[], as?: string) => t.post<MailReadResult>("/api/mail/read", { ids, as }),
    requeue: (id: string, as?: string) =>
      t.post<MailRequeueResult>(letterPath(id, "/requeue"), { as }),
    unread: (as?: string, signal?: AbortSignal) =>
      t.request<MailUnread>("GET", queryPath("/api/mail/unread", { as }), undefined, signal),
    clear: (agent: string) => t.agentPost<MailClearResult>(agent, "mail/clear"),
  };
}
