import { Copy } from "lucide-react";
import { toast } from "sonner";
import { MarkdownContent } from "./TaskMarkdown";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { PRIMARY_FILL } from "./panelStyles";
import { formatTaskTime } from "./taskTime";
import type { TaskComment } from "@/lib/api";

/** Two letters is enough to tell one author from another down a column. */
function initials(author: string): string {
  const name = author.replace(/^[a-z]+:/, "");
  return (name.slice(0, 2) || "··").toUpperCase();
}

const copyComment = async (body: string) => {
  try {
    await navigator.clipboard.writeText(body);
    toast.success("Comment Markdown copied");
  } catch (error) {
    toast.error(`Could not copy comment: ${String(error)}`);
  }
};

/** The thread; `asking` holds the ids of comments with an open question. */
export function TaskCommentList({
  comments,
  asking,
}: {
  comments: TaskComment[];
  asking: ReadonlySet<number>;
}) {
  return (
    /* The grid column is what keeps long words and wide tables inside the
       panel; the vertical rhythm is padding, not a rule between comments. */
    <div className="task-comment-list grid grid-cols-[minmax(0,1fr)]">
      {comments.map((comment) => (
        <article key={comment.id} className="flex min-w-0 gap-2.25 py-1.75">
          <span
            aria-hidden="true"
            className="mt-px grid size-5.5 shrink-0 place-items-center rounded-full bg-muted font-mono text-tiny text-muted-foreground"
          >
            {initials(comment.author)}
          </span>
          <div className="flex min-w-0 flex-1 flex-col gap-0.75">
            <header className="flex min-w-0 items-center gap-2">
              <strong className="shrink-0 font-mono text-meta font-medium">{comment.author}</strong>
              {asking.has(comment.id) && (
                <span
                  className={cn(
                    "inline-flex h-4.25 shrink-0 items-center rounded-chip px-1.5 text-tiny font-medium",
                    PRIMARY_FILL,
                  )}
                >
                  waiting for answer
                </span>
              )}
              <span className="task-comment-actions ml-auto flex shrink-0 items-center gap-1">
                <time
                  className="font-mono text-caption tabular-nums text-muted-foreground"
                  dateTime={comment.created_at}
                  title={new Date(comment.created_at).toLocaleString()}
                >
                  {formatTaskTime(comment.created_at)}
                </time>
                <Button
                  type="button"
                  size="icon-xs"
                  variant="ghost"
                  className="size-5 rounded-sm text-muted-foreground opacity-0 transition-opacity duration-120 ease-[ease] [article:focus-within_&]:opacity-100 [article:hover_&]:opacity-100"
                  aria-label="Copy comment Markdown"
                  title="Copy comment Markdown"
                  onClick={() => void copyComment(comment.body)}
                >
                  <Copy />
                </Button>
              </span>
            </header>
            <MarkdownContent>{comment.body}</MarkdownContent>
          </div>
        </article>
      ))}
      {comments.length === 0 && (
        <p className="pt-0.5 text-ui text-muted-foreground opacity-80">No comments yet.</p>
      )}
    </div>
  );
}
