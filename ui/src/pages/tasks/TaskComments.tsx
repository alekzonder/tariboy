import { useEffect, useState } from "react";
import { COUNT, LABEL, SelectShell } from "./panelStyles";
import { TaskCommentForm } from "./TaskCommentForm";
import { TaskCommentList } from "./TaskCommentList";
import type { TaskComment, TaskPrincipals, TaskWait } from "@/lib/api";
import { idempotencyKey } from "@/lib/ids";

export default function TaskComments({
  comments,
  waits,
  principals,
  assignee,
  formFirst,
  order,
  onOrderChange,
  onComment,
  onDirtyChange,
  onUploadingChange,
}: {
  comments: TaskComment[];
  waits: TaskWait[];
  principals: TaskPrincipals | null;
  assignee: string;
  formFirst: boolean;
  order: "newest" | "oldest";
  onOrderChange: (order: "newest" | "oldest") => void;
  onDirtyChange: (dirty: boolean) => void;
  onComment: (body: string, idempotencyKey: string) => Promise<void>;
  onUploadingChange: (uploading: boolean) => void;
}) {
  const defaultAsk = assignee.replace(/^agent:/, "");
  const [body, setBody] = useState("");
  const [ask, setAsk] = useState({ assignee, principal: defaultAsk });
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  if (ask.assignee !== assignee) setAsk({ assignee, principal: defaultAsk });
  useEffect(() => {
    onUploadingChange(uploading);
  }, [uploading, onUploadingChange]);
  useEffect(() => {
    onDirtyChange(Boolean(body.trim()) || busy || uploading);
  }, [body, busy, uploading, onDirtyChange]);
  const openWaits = waits.filter((wait) => !wait.resolved_at);
  // The wait names the comment that asked, so the chip sits on that comment
  // rather than floating above the whole thread.
  const asking = new Set(openWaits.map((wait) => wait.requesting_comment_id));
  const choices = principals ? [principals.customer, ...principals.agents].filter(Boolean) : [];
  if (defaultAsk && !choices.includes(defaultAsk)) choices.push(defaultAsk);

  const send = async (text: string, resetForm: boolean) => {
    if (busy || uploading) return;
    setBusy(true);
    try {
      await onComment(text, idempotencyKey("comment"));
      if (resetForm) {
        setBody("");
        setAsk({ assignee, principal: defaultAsk });
      }
    } catch {
      // The workspace reports the failure; retain the comment for retry.
    } finally {
      setBusy(false);
    }
  };

  const askText = (text: string) => {
    const principal =
      ask.principal && !ask.principal.includes(":") ? `agent:${ask.principal}` : ask.principal;
    return principal ? `@${principal}\n\n${text}` : text;
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!body.trim()) return;
    await send(askText(body.trim()), true);
  };

  const commentList = <TaskCommentList key="list" comments={comments} asking={asking} />;
  const commentForm = (
    <TaskCommentForm
      key="form"
      body={body}
      onBodyChange={setBody}
      askPrincipal={ask.principal}
      onAskPrincipalChange={(principal) => setAsk({ assignee, principal })}
      choices={choices}
      busy={busy}
      uploading={uploading}
      onUploadingChange={setUploading}
      onSubmit={submit}
      onSendOk={() => void send(askText("Ok"), false)}
    />
  );

  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <div className="flex items-center gap-1.5">
        <span className={LABEL}>Comments</span>
        <span className={COUNT}>{comments.length}</span>
        <label className="ml-auto inline-flex items-center">
          <span className="sr-only">Comment order</span>
          <SelectShell
            aria-label="Comment order"
            className="h-6 pl-2.5 text-meta md:text-meta text-muted-foreground"
            value={order}
            onChange={(event) => onOrderChange(event.target.value as "newest" | "oldest")}
          >
            <option value="newest">Newest first</option>
            <option value="oldest">Oldest first</option>
          </SelectShell>
        </label>
      </div>
      {formFirst ? [commentForm, commentList] : [commentList, commentForm]}
    </section>
  );
}
