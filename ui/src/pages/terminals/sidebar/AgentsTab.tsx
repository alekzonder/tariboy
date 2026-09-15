import { SidebarAgentRow, type AgentRowActions } from "./SidebarAgentRow";
import type { SidebarAgent } from "./sidebarModel";

/**
 * Every agent of every server in one list, ordered the way a chat list is:
 * pinned on top, then whoever is calling for a person. The separator only
 * appears when there is something pinned to separate.
 */
export function AgentsTab({ pinned, rest, actions }: {
  pinned: SidebarAgent[];
  rest: SidebarAgent[];
  actions: AgentRowActions;
}) {
  if (pinned.length === 0 && rest.length === 0) {
    return <div className="px-2 pt-1.5 text-[12.5px] text-muted-foreground">No agents.</div>;
  }
  return (
    <div className="pt-1.5">
      {pinned.map((row) => <SidebarAgentRow key={row.key} row={row} actions={actions} />)}
      {pinned.length > 0 && (
        <div aria-hidden="true" className="mx-3.5 my-2.5 h-px bg-[color-mix(in_oklab,var(--border)_55%,transparent)]" />
      )}
      {rest.map((row) => <SidebarAgentRow key={row.key} row={row} actions={actions} />)}
    </div>
  );
}
