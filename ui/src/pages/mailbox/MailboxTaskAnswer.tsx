import { useState } from "react";
import { SendFilesButton } from "@/components/SendFilesButton";
import { Button } from "@/components/ui/button";
import { idempotencyKey } from "@/lib/ids";
import { cn } from "@/lib/utils";
import { MarkdownEditor, MarkdownModeSegment, type MarkdownMode } from "@/pages/tasks/TaskMarkdown";
import { LABEL, QUIET_ACTION } from "@/pages/tasks/panelStyles";

/**
 * The answer to a task question, written in the letter like a task comment.
 * One idempotency key covers a draft until it is sent, so a retry after a
 * failure cannot post the comment twice; a failure keeps the draft.
 */
export function MailboxTaskAnswer({
  taskKey,
  busy,
  onAnswer,
}: {
  taskKey: string;
  busy: boolean;
  /** Posts the comment; true once it is on the task. */
  onAnswer: (body: string, idempotencyKey: string) => Promise<boolean>;
}) {
  const [body, setBody] = useState("");
  const [mode, setMode] = useState<MarkdownMode>("rich");
  const [uploading, setUploading] = useState(false);
  const [draftKey, setDraftKey] = useState(() => idempotencyKey("answer"));
  const disabled = busy || uploading;

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const text = body.trim();
    if (!text || disabled) return;
    if (!(await onAnswer(text, draftKey))) return;
    setBody("");
    setDraftKey(idempotencyKey("answer"));
  };

  return (
    <form
      aria-label={`Answer on ${taskKey}`}
      className="grid grid-cols-[minmax(0,1fr)] gap-2 rounded-md bg-muted p-3"
      onSubmit={(event) => void submit(event)}
    >
      <div>
        <div className="flex items-center gap-2 pb-1.75">
          <label htmlFor="mail-task-answer" className={LABEL}>
            Answer
          </label>
          <MarkdownModeSegment
            label="Answer editing mode"
            mode={mode}
            onModeChange={setMode}
            disabled={busy}
          />
          <SendFilesButton
            disabled={busy}
            onUploadingChange={setUploading}
            className={cn(QUIET_ACTION, "ml-auto border-0 bg-transparent")}
            onUploaded={(paths) =>
              setBody(
                (current) =>
                  current + (current && !current.endsWith("\n") ? "\n" : "") + paths.join("\n"),
              )
            }
          />
        </div>
        <MarkdownEditor
          id="mail-task-answer"
          placeholder="Answer"
          value={body}
          onChange={setBody}
          disabled={busy}
          mode={mode}
          surface="card"
        />
      </div>
      <div className="flex justify-end">
        <Button type="submit" size="sm" disabled={disabled || !body.trim()}>
          Send answer
        </Button>
      </div>
    </form>
  );
}
