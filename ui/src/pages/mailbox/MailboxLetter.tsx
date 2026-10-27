import { ArrowLeft } from "lucide-react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { useApiQuery } from "@/hooks/useApiQuery";
import type { Letter, MailBox } from "@/lib/api";
import { letterTaskKey } from "@/lib/letters";
import { MarkdownContent } from "@/pages/tasks/TaskMarkdown";
import { mailKeys } from "./mailQueries";
import { letterActions, letterState, principalName, type MailboxMode } from "./mailboxModel";

export interface LetterHandlers {
  onReply: (letter: Letter) => void;
  onArchive: (letter: Letter) => void;
  onRequeue: (letter: Letter) => void;
  /** Opens the task drawer over the mailbox. */
  onOpenTask: (key: string) => void;
  busy: boolean;
}

function ThreadLine({ id, as }: { id: string; as: string }) {
  const api = useApi();
  const parent = useApiQuery({
    queryKey: [...mailKeys.server(api.target), as, "letter", id],
    queryFn: ({ signal }) => api.mail.show(id, as || undefined, signal),
  });
  const subject = parent.data?.letter.subject;
  return (
    <p className="text-meta text-muted-foreground">
      In reply to {subject ? `“${subject}”` : "an earlier letter"}
    </p>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-2 text-meta">
      <dt className="w-16 shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  );
}

function LetterActionBar({
  letter,
  box,
  mode,
  handlers,
}: {
  letter: Letter;
  box: MailBox;
  mode: MailboxMode;
  handlers: LetterHandlers;
}) {
  const actions = letterActions(letter, box, mode);
  const key = letterTaskKey(letter);
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {actions.reply && (
        <Button size="sm" onClick={() => handlers.onReply(letter)} disabled={handlers.busy}>
          Reply
        </Button>
      )}
      {actions.answerOnTask && (
        <Button size="sm" onClick={() => handlers.onOpenTask(key)}>
          Answer on the task
        </Button>
      )}
      {actions.archive && (
        <Button
          size="sm"
          variant="outline"
          onClick={() => handlers.onArchive(letter)}
          disabled={handlers.busy}
        >
          Archive
        </Button>
      )}
      {actions.requeue && (
        <Button
          size="sm"
          variant="outline"
          onClick={() => handlers.onRequeue(letter)}
          disabled={handlers.busy}
        >
          Requeue
        </Button>
      )}
      {actions.taskLink && (
        <Button size="sm" variant="ghost" onClick={() => handlers.onOpenTask(key)}>
          Open {key}
        </Button>
      )}
    </div>
  );
}

/** The selected letter: its header, thread line, body and the actions it allows. */
export function MailboxLetter({
  letter,
  box,
  mode,
  as,
  handlers,
  onBack,
}: {
  letter: Letter;
  box: MailBox;
  mode: MailboxMode;
  as: string;
  handlers: LetterHandlers;
  /** Shown on narrow screens, where the letter replaces the list. */
  onBack?: () => void;
}) {
  return (
    <article aria-label={letter.subject || "Letter"} className="flex flex-col gap-3 px-4 py-3">
      {onBack && (
        <Button size="sm" variant="ghost" className="self-start" onClick={onBack}>
          <ArrowLeft className="size-3.5" aria-hidden />
          Back
        </Button>
      )}
      <h2 className="text-body font-medium">{letter.subject || "(no subject)"}</h2>
      <dl className="flex flex-col gap-0.5">
        <Field label="From">{principalName(letter.from)}</Field>
        <Field label="To">{principalName(letter.to)}</Field>
        <Field label="Received">{new Date(letter.created_at).toLocaleString()}</Field>
        <Field label="State">{letterState(letter)}</Field>
      </dl>
      {letter.in_reply_to && <ThreadLine id={letter.in_reply_to} as={as} />}
      <LetterActionBar letter={letter} box={box} mode={mode} handlers={handlers} />
      <div className="border-t pt-3">
        <MarkdownContent>{letter.body}</MarkdownContent>
      </div>
    </article>
  );
}
