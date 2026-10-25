import { NavLink } from "react-router-dom";
import type { RefObject } from "react";
import { cn } from "@/lib/utils";
import type { AgentTabEntry } from "./agentWorkspaceModel";

type AgentWorkspaceTabsProps = {
  tabs: readonly AgentTabEntry[];
  base: string;
  agentName: string;
  unreadChat: number;
  questionTasks: number;
  stripRef: RefObject<HTMLElement | null>;
};

function ChatUnreadDot({ count, agentName }: { count: number; agentName: string }) {
  return (
    <span
      role="img"
      aria-label={`${count} unread message${count === 1 ? "" : "s"} from ${agentName}`}
      title={`${count} unread message${count === 1 ? "" : "s"}`}
      className="ml-1.5 size-1.25 rounded-full bg-primary"
    />
  );
}

function TaskQuestionsBadge({ count }: { count: number }) {
  return (
    <span
      role="img"
      aria-label={`${count} task${count === 1 ? "" : "s"} with unread agent questions`}
      title={`${count} task${count === 1 ? "" : "s"} with unread agent questions`}
      className="ml-1.5 rounded-full bg-primary px-1.5 text-caption font-medium leading-4 text-primary-foreground"
    >
      {count}
    </span>
  );
}

export function AgentWorkspaceTabs({
  tabs,
  base,
  agentName,
  unreadChat,
  questionTasks,
  stripRef,
}: AgentWorkspaceTabsProps) {
  return (
    <nav
      ref={stripRef}
      aria-label="Agent workspace"
      className="scrollbar-none flex items-center gap-0.5 overflow-x-auto border-b px-3"
    >
      {tabs.map(([key, label]) => (
        <NavLink
          key={key}
          to={`${base}/${key}`}
          className={({ isActive }) =>
            cn(
              "flex h-8.5 shrink-0 items-center border-b-2 border-transparent px-2.5 text-body whitespace-nowrap text-muted-foreground hover:text-foreground",
              isActive && "border-primary font-medium text-foreground",
            )
          }
        >
          {label}
          {key === "chat" && unreadChat > 0 && (
            <ChatUnreadDot count={unreadChat} agentName={agentName} />
          )}
          {key === "tasks" && questionTasks > 0 && <TaskQuestionsBadge count={questionTasks} />}
        </NavLink>
      ))}
    </nav>
  );
}
