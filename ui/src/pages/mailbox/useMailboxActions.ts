import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useApi } from "@/components/apiContext";
import { ApiError, type Letter } from "@/lib/api";
import { letterTaskKey } from "@/lib/letters";
import { writeTask } from "@/pages/tasks/taskQueries";
import type { ComposeValues } from "./MailboxCompose";

const message = (error: unknown) => (error instanceof ApiError ? error.message : String(error));

/** The mailbox writes, acting as `as` ("" = the customer); a new letter is
 *  always the customer's. Each one toasts its result and refetches; nothing
 *  changes optimistically. */
export function useMailboxActions(as: string, refresh: () => Promise<void>) {
  const api = useApi();
  const client = useQueryClient();
  const [busy, setBusy] = useState(false);
  const actAs = as || undefined;

  const run = async (done: string, action: string, write: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await write();
      toast.success(done);
      return true;
    } catch (error) {
      toast.error(`${action} failed: ${message(error)}`);
      return false;
    } finally {
      await refresh();
      setBusy(false);
    }
  };

  return {
    busy,
    send: (values: ComposeValues) =>
      run("sent", "send", () =>
        api.mail.send({
          to: [values.to],
          subject: values.subject,
          text: values.text,
          notice: values.notice,
        }),
      ),
    reply: (letter: Letter, text: string) =>
      run("replied", "reply", () => api.mail.reply(letter.id, { text, as: actAs })),
    // The customer's comment answers the question: the daemon closes the
    // letter, and the task's status and comments refetch.
    answer: (letter: Letter, body: string, key: string) =>
      run("answered", "answer", () =>
        writeTask(client, api, () => api.tasks.addTaskComment(letterTaskKey(letter), body, key)),
      ),
    archive: (letter: Letter) =>
      run("archived", "archive", () => api.mail.close(letter.id, { as: actAs })),
    requeue: (letter: Letter) =>
      run("requeued", "requeue", () => api.mail.requeue(letter.id, actAs)),
    // Reading is quiet: a failure leaves the letter unread for the next open.
    read: async (letter: Letter) => {
      try {
        await api.mail.read([letter.id], actAs);
      } catch {
        /* Left unread for the next open. */
      } finally {
        await refresh();
      }
    },
  };
}
