import { Fragment } from "react";
import { ArrowDown, MessageSquare } from "lucide-react";
import type { ChatMessage } from "@/lib/api";
import { cn } from "@/lib/utils";
import ChatMessageRow from "./ChatMessage";
import { DayDivider, UnreadDivider } from "./ChatDivider";
import type { AgentChatState } from "./useAgentChat";
import type { useChatScroll } from "./useChatScroll";

type FeedChat = Pick<
  AgentChatState,
  "name" | "messages" | "shown" | "openQuestion" | "newCount" | "markRead" | "reply"
>;

export function ChatFeed({
  chat,
  scroll,
  onOpenTask,
  onCreateTask,
}: {
  chat: FeedChat;
  scroll: ReturnType<typeof useChatScroll>;
  onOpenTask: (key: string) => void;
  onCreateTask: (message: ChatMessage) => void;
}) {
  const { name, messages, shown, openQuestion, newCount, markRead, reply } = chat;
  const { bottom, unreadRule, atBottom, onScroll } = scroll;
  return (
    <>
      <div
        onScroll={onScroll}
        className="flex min-h-0 flex-1 flex-col overflow-auto border-t pt-0.5 pb-2.5"
      >
        {messages.length === 0 && (
          <div className="flex flex-1 flex-col items-center justify-center gap-1.5 px-6 py-10 text-center">
            <span className="grid size-8 place-items-center rounded-panel bg-muted text-muted-foreground">
              <MessageSquare className="size-4" aria-hidden />
            </span>
            <span className="text-body font-medium">No messages yet</span>
            <span className="max-w-75 text-ui leading-[1.5] text-pretty text-muted-foreground">
              Write to this agent — it answers here and can open tasks straight from the thread.
            </span>
          </div>
        )}
        {shown.map((item) => (
          <Fragment key={item.kind === "message" ? item.message.id : item.id}>
            {item.kind === "day" && <DayDivider label={item.label} />}
            {item.kind === "unread" && (
              <div ref={unreadRule}>
                <UnreadDivider label={item.label} onMarkRead={markRead} />
              </div>
            )}
            {item.kind === "message" && (
              <ChatMessageRow
                item={item}
                onOpenTask={onOpenTask}
                onReply={reply}
                onCreateTask={(entry) => onCreateTask(entry.message)}
              />
            )}
          </Fragment>
        ))}
        {openQuestion && (
          <div className="flex items-center gap-2.25 px-4 pt-1 pl-3.5">
            <span className="grid size-5.5 place-items-center rounded-control bg-[color-mix(in_oklab,var(--status-running)_16%,transparent)]">
              <span className="size-1.25 rounded-full bg-[var(--status-running)]" />
            </span>
            <span className="text-meta text-muted-foreground">
              {name} is waiting for an answer…
            </span>
          </div>
        )}
        <div ref={bottom} />
      </div>
      {newCount > 0 && !atBottom && (
        <div className="relative z-2 flex h-0 shrink-0 justify-center">
          <button
            type="button"
            onClick={() => unreadRule.current?.scrollIntoView({ block: "center" })}
            className={cn(
              "absolute bottom-1.5 flex h-6.5 items-center gap-1.5 rounded-full bg-primary px-2.5",
              "text-meta font-medium text-primary-foreground shadow-[var(--lift)]",
            )}
          >
            <ArrowDown className="size-3" aria-hidden />
            {newCount} new
          </button>
        </div>
      )}
    </>
  );
}
