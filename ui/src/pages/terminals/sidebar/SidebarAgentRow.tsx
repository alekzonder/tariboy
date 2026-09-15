import { useDraggable, useDroppable } from "@dnd-kit/core";
import {
  ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { AgentRow } from "@/components/AgentRow";
import { cn } from "@/lib/utils";
import type { SidebarAgent } from "./sidebarModel";

/** What every agent row needs to know, wherever the three tabs place it. */
export interface AgentRowActions {
  selected?: { hostId: string; agent: string };
  attention: ReadonlySet<string>;
  pinned: ReadonlySet<string>;
  onSelect: (hostId: string, agent: string) => void;
  onClone: (hostId: string, agent: string) => void;
  onTogglePin: (key: string) => void;
}

/* @dnd-kit exposes callback refs and live attributes from its hooks for
 * render-time spreading. */
/* eslint-disable react-hooks/refs */
function DraggableRow({ dragKey, ...props }: React.ComponentProps<typeof AgentRow> & {
  dragKey: string;
}) {
  const disabled = props.disabled;
  const drag = useDraggable({ id: dragKey, disabled });
  const drop = useDroppable({ id: dragKey, disabled });
  return <AgentRow
    ref={(node) => { drag.setNodeRef(node); drop.setNodeRef(node); }}
    style={{ opacity: drag.isDragging ? 0.45 : undefined }}
    {...props}
    className={cn("cursor-grab active:cursor-grabbing", drop.isOver && "ring-1 ring-primary", props.className)}
    {...drag.attributes}
    {...drag.listeners}
  />;
}
/* eslint-enable react-hooks/refs */

/**
 * One agent in any of the three tabs: the shared row, plus the context menu
 * that owns per-agent actions. `dragKey` turns it into a drag source; only the
 * Servers tab, which is the one with a persisted order, passes it.
 */
export function SidebarAgentRow({ row, actions, dragKey }: {
  row: SidebarAgent;
  actions: AgentRowActions;
  dragKey?: string;
}) {
  const { agent } = row;
  const pinned = actions.pinned.has(row.key);
  const disabled = Boolean(row.hostError);
  const rowProps = {
    name: agent.name,
    state: agent.state,
    outOfBudget: Boolean(agent.budget?.exhausted?.length),
    selected: actions.selected?.hostId === row.hostId && actions.selected.agent === agent.name,
    unread: actions.attention.has(row.key),
    unreadLabel: `Unread customer question for ${agent.name} on ${row.hostLabel}`,
    unreadMessages: row.chat?.unread ?? 0,
    interactive: agent.interactive !== false,
    "aria-label": `Open ${agent.name}`,
    "data-testid": "sidebar-agent",
    disabled,
    onClick: () => { if (!disabled) actions.onSelect(row.hostId, agent.name); },
  };
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div className="w-full">
          {dragKey
            ? <DraggableRow dragKey={dragKey} {...rowProps} />
            : <AgentRow {...rowProps} />}
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem onSelect={() => actions.onTogglePin(row.key)}>
          {pinned ? "Unpin" : "Pin"}
        </ContextMenuItem>
        <ContextMenuItem disabled={disabled} onSelect={() => actions.onClone(row.hostId, agent.name)}>
          Clone
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}
