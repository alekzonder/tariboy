import { useState, type Dispatch, type SetStateAction } from "react";
import { MarkdownEditor, MarkdownModeSegment, type MarkdownMode } from "./TaskMarkdown";
import { Button } from "@/components/ui/button";
import { SendFilesButton } from "@/components/SendFilesButton";
import { cn } from "@/lib/utils";
import { LABEL, QUIET_ACTION, SelectShell } from "./panelStyles";

/** The comment composer: who to ask, the Markdown body, attachments, and the
 *  two ways to post. The draft itself is owned by `TaskComments`. */
export function TaskCommentForm({
  body,
  onBodyChange,
  askPrincipal,
  onAskPrincipalChange,
  choices,
  busy,
  uploading,
  onUploadingChange,
  onSubmit,
  onSendOk,
}: {
  body: string;
  onBodyChange: Dispatch<SetStateAction<string>>;
  askPrincipal: string;
  onAskPrincipalChange: (principal: string) => void;
  choices: string[];
  busy: boolean;
  uploading: boolean;
  onUploadingChange: (uploading: boolean) => void;
  onSubmit: (event: React.FormEvent) => Promise<void>;
  onSendOk: () => void;
}) {
  const [mode, setMode] = useState<MarkdownMode>("rich");
  return (
    <form
      className="grid grid-cols-[minmax(0,1fr)] gap-2 rounded-md bg-muted p-3"
      onSubmit={(event) => void onSubmit(event)}
    >
      <label className="flex min-w-0 items-center gap-2">
        <span className={LABEL}>Ask</span>
        <SelectShell
          aria-label="Ask"
          className="h-6 bg-card pl-2.5 text-meta md:text-meta"
          value={askPrincipal}
          onChange={(event) => onAskPrincipalChange(event.target.value)}
        >
          <option value="">No explicit answer needed</option>
          {choices.map((principal) => (
            <option key={principal} value={principal}>
              {principal}
            </option>
          ))}
        </SelectShell>
      </label>
      <div>
        <div className="flex items-center gap-2 pb-1.75">
          <label htmlFor="task-comment" className={LABEL}>
            Comment
          </label>
          <MarkdownModeSegment
            label="Comment editing mode"
            mode={mode}
            onModeChange={setMode}
            disabled={busy}
          />
          <SendFilesButton
            disabled={busy}
            onUploadingChange={onUploadingChange}
            className={cn(QUIET_ACTION, "ml-auto border-0 bg-transparent")}
            onUploaded={(paths) =>
              onBodyChange(
                (current) =>
                  current + (current && !current.endsWith("\n") ? "\n" : "") + paths.join("\n"),
              )
            }
          />
        </div>
        <MarkdownEditor
          id="task-comment"
          placeholder="Comment"
          value={body}
          onChange={onBodyChange}
          disabled={busy}
          mode={mode}
          surface="card"
        />
      </div>
      <div className="flex justify-end gap-1.5">
        {/* Secondary, not ghost: it posts a comment, so it stays a button. */}
        {!body.trim() && (
          <Button
            type="button"
            size="sm"
            variant="secondary"
            className="h-7 rounded-control"
            disabled={busy || uploading}
            onClick={onSendOk}
          >
            Send Ok
          </Button>
        )}
        <Button type="submit" size="sm" disabled={busy || uploading || !body.trim()}>
          Send comment
        </Button>
      </div>
    </form>
  );
}
