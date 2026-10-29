import { NavLink } from "react-router-dom";
import type { RefObject } from "react";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { cn } from "@/lib/utils";
import type { AgentTabEntry } from "./agentWorkspaceModel";

type AgentWorkspaceTabsProps = {
  tabs: readonly AgentTabEntry[];
  base: string;
  questionTasks: number;
  /** The agent's open inbox letters. */
  openLetters: number;
  stripRef: RefObject<HTMLElement | null>;
};

function CountBadge({ count, label }: { count: number; label: string }) {
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className="ml-1.5 rounded-full bg-primary px-1.5 text-caption font-medium leading-4 text-primary-foreground"
    >
      {count}
    </span>
  );
}

export function AgentWorkspaceTabs({
  tabs,
  base,
  questionTasks,
  openLetters,
  stripRef,
}: AgentWorkspaceTabsProps) {
  return (
    <nav
      ref={stripRef}
      aria-label="Agent workspace"
      data-tour={TOUR_ANCHORS.agentTabs}
      className="scrollbar-none flex items-center gap-0.5 overflow-x-auto border-b px-3"
    >
      {tabs.map(([key, label]) => (
        <NavLink
          key={key}
          to={`${base}/${key}`}
          data-tour={key === "configuration" ? TOUR_ANCHORS.agentConfiguration : undefined}
          className={({ isActive }) =>
            cn(
              "flex h-8.5 shrink-0 items-center border-b-2 border-transparent px-2.5 text-body whitespace-nowrap text-muted-foreground hover:text-foreground",
              isActive && "border-primary font-medium text-foreground",
            )
          }
        >
          {label}
          {key === "tasks" && questionTasks > 0 && (
            <CountBadge
              count={questionTasks}
              label={`${questionTasks} open agent question${questionTasks === 1 ? "" : "s"}`}
            />
          )}
          {key === "mailbox" && openLetters > 0 && (
            <CountBadge
              count={openLetters}
              label={`${openLetters} open letter${openLetters === 1 ? "" : "s"}`}
            />
          )}
        </NavLink>
      ))}
    </nav>
  );
}
