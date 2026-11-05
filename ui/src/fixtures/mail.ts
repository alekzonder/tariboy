import type { Letter } from "@/types";

import { minutesAgo } from "./time";

/** An open letter to the user: not answered, not dead. */
export const letter = (over: Partial<Letter> = {}): Letter => ({
  id: "m-1",
  seq: 1,
  from: "agent:docs-bot",
  to: "user:owner",
  kind: "question",
  subject: "Which image builds the docs: worker:v1 or bare:latest?",
  body: "",
  data: {},
  needs_reply: true,
  in_reply_to: "",
  answer_id: "",
  answered_at: "",
  close_note: "",
  attempts: 1,
  deadline: "",
  dead_at: "",
  dead_reason: "",
  instant: false,
  instant_at: "",
  instant_iteration: "",
  instant_state: "",
  produced_in_iteration: "",
  read_at: "",
  created_at: minutesAgo(45),
  ...over,
});
