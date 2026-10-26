import type { Letter } from "@/lib/api";

/** One letter with every field the spec requires; override what a test reads. */
export function letter(over: Partial<Letter> = {}): Letter {
  return {
    id: "m1",
    seq: 1,
    from: "agent:dev",
    to: "user:customer",
    subject: "Hello",
    body: "Body",
    kind: "message",
    data: {},
    needs_reply: true,
    in_reply_to: "",
    deadline: "",
    created_at: "2026-10-26T09:00:00Z",
    read_at: "",
    answered_at: "",
    answer_id: "",
    close_note: "",
    dead_at: "",
    dead_reason: "",
    attempts: 0,
    produced_in_iteration: "",
    ...over,
  };
}
