export type { Letter, MailList, MailUnread } from "@/lib/api-types.generated";

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
  /** Deliver into the agent's running iteration after its current tool call. */
  instant?: boolean;
  as?: string;
}
export interface MailReplyInput {
  text: string;
  ask?: boolean;
  instant?: boolean;
  as?: string;
}
