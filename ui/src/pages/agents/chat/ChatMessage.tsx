import { Check, CheckCheck, CornerUpLeft, Copy, ListPlus } from "lucide-react";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import ChatMarkdown from "./ChatMarkdown";
import { principalName, shortTime, taskKeysOf, type FeedMessage } from "./chatFeed";

/** The receipt glyph on the customer's own messages. `sent` is one tick while
 *  the publish is in flight, `delivered` two quiet ticks once the daemon has
 *  it, `read` two primary ticks once the agent has spoken after it. */
function ReceiptMark({ item }: { item: FeedMessage }) {
  if (!item.receipt) return null;
  return (
    <div className="flex items-center gap-1.5 pt-0.5 text-tiny text-muted-foreground">
      {item.receipt === "sent" && <Check className="size-3 opacity-60" aria-hidden />}
      {item.receipt === "delivered" && <CheckCheck className="size-3 opacity-70" aria-hidden />}
      {item.receipt === "read" && <CheckCheck className="size-3 text-primary" aria-hidden />}
      <span>{item.readLabel ?? (item.receipt === "sent" ? "Sending" : "Delivered")}</span>
    </div>
  );
}

export default function ChatMessage({
  item,
  onOpenTask,
  onReply,
  onCreateTask,
}: {
  item: FeedMessage;
  onOpenTask: (key: string) => void;
  onReply: (item: FeedMessage) => void;
  onCreateTask: (item: FeedMessage) => void;
}) {
  const { message, mine, head } = item;
  const author = principalName(message.from);
  const role = mine ? "you" : message.type || "agent";
  // A key the daemon attached rather than one written in the text: the text's
  // own keys are already links inside the rendered Markdown.
  const attached = taskKeysOf(message).filter((key) => !message.text.includes(key));
  const actions = [
    { label: "Reply to this message", icon: CornerUpLeft, run: () => onReply(item) },
    {
      label: "Copy message text",
      icon: Copy,
      run: () =>
        void navigator.clipboard
          ?.writeText(message.text)
          .catch(() => toast.error("Could not copy the message")),
    },
    { label: "Create a task from this message", icon: ListPlus, run: () => onCreateTask(item) },
  ];
  return (
    <div className="group relative flex gap-2.25 py-px pr-3.5 pl-3 hover:bg-muted">
      <div className="flex w-5.5 shrink-0 justify-center pt-0.5">
        {head ? (
          <span
            className={cn(
              "grid size-5.5 place-items-center text-tiny font-semibold uppercase",
              mine
                ? "rounded-full bg-secondary text-muted-foreground"
                : "rounded-control bg-[color-mix(in_oklab,var(--status-running)_16%,transparent)] font-mono text-[var(--status-running)]",
            )}
          >
            {author.charAt(0)}
          </span>
        ) : (
          <span className="pt-0.75 font-mono text-micro text-muted-foreground opacity-0 tabular-nums group-hover:opacity-55">
            {shortTime(message.ts)}
          </span>
        )}
      </div>
      <div className="min-w-0 flex-1 pb-px">
        {head && (
          <div className="flex min-w-0 items-baseline gap-1.75 pb-px">
            <span className="truncate text-body-sm font-semibold">{author}</span>
            <span className="inline-flex h-3.75 shrink-0 items-center rounded-chip bg-muted px-1.25 font-mono text-micro text-muted-foreground">
              {role}
            </span>
            <span
              className="shrink-0 font-mono text-tiny text-muted-foreground tabular-nums"
              title={message.ts}
            >
              {shortTime(message.ts)}
            </span>
          </div>
        )}
        <ChatMarkdown onOpenTask={onOpenTask}>{message.text}</ChatMarkdown>
        {attached.length > 0 && (
          <div className="flex flex-wrap gap-1.5 pt-1">
            {attached.map((key) => (
              <button
                key={key}
                type="button"
                onClick={() => onOpenTask(key)}
                className="rounded-chip border px-1.5 font-mono text-tiny hover:bg-primary/10"
              >
                {key}
              </button>
            ))}
          </div>
        )}
        <ReceiptMark item={item} />
      </div>
      <div className="absolute top-0 right-2 hidden items-center gap-0.5 rounded-control border bg-card p-0.5 group-focus-within:flex group-hover:flex">
        {actions.map(({ label, icon: Icon, run }) => (
          <button
            key={label}
            type="button"
            aria-label={label}
            title={label}
            onClick={run}
            className="grid size-5.5 place-items-center rounded-chip text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <Icon className="size-3.5" />
          </button>
        ))}
      </div>
    </div>
  );
}
