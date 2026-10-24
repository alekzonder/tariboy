import type { ReactNode } from "react";
import { Check, Mail, Plus, Search } from "lucide-react";
import { KINDS } from "./chatKinds";
import { ChatExportButton } from "./ChatExportButton";
import { ChatTypesMenu } from "./ChatTypesMenu";
import type { AgentChatState } from "./useAgentChat";

type ToolbarChat = Pick<
  AgentChatState,
  | "name"
  | "messages"
  | "shared"
  | "current"
  | "selected"
  | "setSelected"
  | "query"
  | "setQuery"
  | "newCount"
  | "markRead"
  | "markUnread"
  | "types"
  | "toggleType"
  | "resetTypes"
>;

export function ChatToolbar({
  chat,
  leading,
  personalOnly,
  onCreate,
}: {
  chat: ToolbarChat;
  leading?: ReactNode;
  personalOnly: boolean;
  onCreate: () => void;
}) {
  const {
    name,
    messages,
    shared,
    current,
    selected,
    setSelected,
    query,
    setQuery,
    newCount,
    markRead,
    markUnread,
    types,
    toggleType,
    resetTypes,
  } = chat;

  return (
    <div className="flex h-12 shrink-0 items-center gap-2 pr-3 pl-4">
      {leading}
      {!personalOnly && (
        <div
          role="tablist"
          aria-label="Agent chats"
          className="flex shrink-0 gap-0.5 rounded-panel bg-muted p-0.5"
        >
          {[
            ...KINDS.map(([key, label]) => [key, label] as [string, string]),
            ...shared.map((chat) => [chat.id, chat.title || chat.id] as [string, string]),
          ].map(([key, label]) => (
            <button
              key={key}
              type="button"
              role="tab"
              aria-selected={selected === key}
              onClick={() => setSelected(key)}
              className={`h-5.5 max-w-35 truncate rounded-control px-2.25 text-meta ${
                selected === key
                  ? "bg-card font-medium text-foreground shadow-[var(--raise)]"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {label}
            </button>
          ))}
          <button
            type="button"
            aria-label="New chat"
            title="Start a chat with this agent and others"
            onClick={onCreate}
            className="grid size-5.5 place-items-center rounded-control text-muted-foreground hover:bg-card hover:text-foreground"
          >
            <Plus className="size-3.5" aria-hidden />
          </button>
        </div>
      )}
      <label className="flex h-7 w-58 items-center gap-1.75 rounded-md bg-muted px-2.25 text-muted-foreground">
        <Search className="size-3 shrink-0" aria-hidden />
        <input
          type="search"
          aria-label="Search messages"
          placeholder="Search messages"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="w-full bg-transparent text-ui outline-none"
        />
      </label>
      <span className="truncate text-meta text-muted-foreground">
        {/* A shared chat says who is in it: in a multi-agent conversation the
            author of a message is not deducible from the thread it is in. */}
        {current
          ? (current.participants ?? []).map((principal) => principal.split(":")[1]).join(", ")
          : messages.length === 0
            ? "no messages"
            : `you + ${name} · ${messages.length} message${messages.length === 1 ? "" : "s"}`}
      </span>
      <div className="ml-auto flex items-center gap-1">
        {newCount > 0 ? (
          <button
            type="button"
            onClick={markRead}
            className="flex h-6.5 items-center gap-1.5 rounded-control bg-primary/12 px-2.5 text-ui font-medium text-primary hover:bg-primary/18"
          >
            <Check className="size-3.5" aria-hidden />
            Mark read
          </button>
        ) : (
          <button
            type="button"
            onClick={() => void markUnread()}
            disabled={messages.length === 0}
            title="Mark this chat unread"
            className="flex h-6.5 items-center gap-1.5 rounded-control px-2.5 text-ui text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
          >
            <Mail className="size-3.5" aria-hidden />
            Mark unread
          </button>
        )}
        <ChatExportButton messages={messages} name={name} />
        <ChatTypesMenu types={types} toggleType={toggleType} resetTypes={resetTypes} />
      </div>
    </div>
  );
}
