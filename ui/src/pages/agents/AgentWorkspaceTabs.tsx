import { NavLink } from "react-router-dom";
import type { RefObject } from "react";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { cn } from "@/lib/utils";
import { AGENT_TABS } from "./agentWorkspaceModel";

type AgentWorkspaceTabsProps = {
  base: string;
  /** What the agent waits on the user for; shown on Work. */
  needsCount: number;
  stripRef: RefObject<HTMLElement | null>;
};

export function AgentWorkspaceTabs({ base, needsCount, stripRef }: AgentWorkspaceTabsProps) {
  return (
    <nav
      ref={stripRef}
      aria-label="Agent workspace"
      data-tour={TOUR_ANCHORS.agentTabs}
      className="scrollbar-none flex items-center gap-0.5 overflow-x-auto border-b px-3"
    >
      {AGENT_TABS.map(([key, label]) => (
        <NavLink
          key={key}
          to={`${base}/${key}`}
          data-tour={key === "settings" ? TOUR_ANCHORS.agentConfiguration : undefined}
          className={({ isActive }) =>
            cn(
              "flex h-8.5 shrink-0 items-center border-b-2 border-transparent px-2.5 text-body whitespace-nowrap text-muted-foreground hover:text-foreground",
              isActive && "border-primary font-medium text-foreground",
            )
          }
        >
          {label}
          {key === "work" && needsCount > 0 && (
            <span
              role="img"
              aria-label={`${needsCount} waiting for you`}
              title={`${needsCount} waiting for you`}
              className="ml-1.5 rounded-full bg-primary px-1.5 text-caption font-medium leading-4 text-primary-foreground"
            >
              {needsCount}
            </span>
          )}
        </NavLink>
      ))}
    </nav>
  );
}
