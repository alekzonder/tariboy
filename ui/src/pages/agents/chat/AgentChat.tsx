import { useState, type ReactNode } from "react";
import type { ChatMessage } from "@/lib/api";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import ChatComposer from "./ChatComposer";
import ChatTaskFromMessage from "./ChatTaskFromMessage";
import ChatCreate from "./ChatCreate";
import { ChatToolbar } from "./ChatToolbar";
import { ChatFeed } from "./ChatFeedView";
import { useAgentChat } from "./useAgentChat";
import { useChatScroll } from "./useChatScroll";

/**
 * The conversation with one agent: the customer's messages in that agent's
 * inbox merged with the agent's messages in the customer's own channel. The
 * daemon owns that projection; this view renders it, sends into it, and marks
 * it read. Live updates arrive over the host's message socket, which is a hint
 * to refetch — the HTTP response stays authoritative.
 *
 * There is no conversation list here: the conversation is whichever agent the
 * sidebar has selected, one agent to one thread.
 */
export default function AgentChat({ hostId = "", leading, personalOnly = false }: {
  hostId?: string;
  /** Simple mode: the conversation only, without the chat tablist. */
  personalOnly?: boolean;
  /** Rendered at the left of the toolbar by whoever owns the section switch,
   *  so the tab keeps one toolbar row instead of two. */
  leading?: ReactNode;
}) {
  const chat = useAgentChat(hostId);
  const {
    name, target, draft, setDraft, sending, writable, openQuestion, selected,
    setSelected, send, loadShared,
  } = chat;
  const [creating, setCreating] = useState(false);
  const [taskKey, setTaskKey] = useState("");
  const [taskFrom, setTaskFrom] = useState<ChatMessage | null>(null);
  const scroll = useChatScroll(chat.shown.length);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <ChatToolbar
        chat={chat}
        leading={leading}
        personalOnly={personalOnly}
        onCreate={() => setCreating(true)}
      />
      <ChatFeed
        chat={chat}
        scroll={scroll}
        onOpenTask={setTaskKey}
        onCreateTask={setTaskFrom}
      />
      {writable ? (
        <ChatComposer
          agent={name}
          target={target}
          value={draft}
          onChange={setDraft}
          onSend={() => void send()}
          sending={sending}
          hint={openQuestion ? "Answers the agent's open question" : undefined}
        />
      ) : (
        <p className="border-t px-4 py-3 text-[12px] text-muted-foreground">
          {selected === "tasks"
            ? `Task notifications addressed to ${name}. Answer a task in its own drawer.`
            : `Service wake-ups for ${name}: script results, goals and schedules.`}
        </p>
      )}
      {taskKey && (
        <TaskDrawer
          key={taskKey}
          taskKey={taskKey}
          target={target}
          onClose={() => setTaskKey("")}
        />
      )}
      {creating && (
        <ChatCreate
          agent={name}
          target={target}
          onClose={() => setCreating(false)}
          onCreated={(chat) => { setCreating(false); setSelected(chat); void loadShared(); }}
        />
      )}
      {taskFrom && (
        <ChatTaskFromMessage
          message={taskFrom}
          agent={name}
          target={target}
          onClose={() => setTaskFrom(null)}
          onCreated={(key) => { setTaskFrom(null); setTaskKey(key); }}
        />
      )}
    </div>
  );
}
