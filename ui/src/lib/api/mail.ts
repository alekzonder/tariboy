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
import type { MailListQuery, MailReplyInput, MailSendInput } from "@/types/mail";
import type { Transport } from "./client";
import { queryPath } from "./tasks";

export type * from "@/types/mail";

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
