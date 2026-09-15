import type { ButtonHTMLAttributes } from "react";
import { useDraggable, useDroppable } from "@dnd-kit/core";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { HostStatus } from "@/components/HostStatus";
import { cn } from "@/lib/utils";
import type { HostAgents } from "@/lib/aggregate";
import { hostUpdateAvailable, type DaemonMeta } from "@/lib/daemons";
import { dragId } from "./sidebarDnd";
import {
  SECTION_ICON_CLASS, SECTION_TITLE_CLASS, SectionEmpty, SidebarSectionHeader,
} from "./SidebarSectionHeader";
import { SidebarAgentRow, type AgentRowActions } from "./SidebarAgentRow";
import { hostAgents, ordered } from "./sidebarModel";

/* @dnd-kit exposes callback refs and live attributes from its hooks for
 * render-time spreading. */
/* eslint-disable react-hooks/refs */
function SortableButton({ dragKey, disabled, style, className, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & {
  dragKey: string;
}) {
  const drag = useDraggable({ id: dragKey, disabled });
  const drop = useDroppable({ id: dragKey, disabled });
  return <button
    ref={(node) => { drag.setNodeRef(node); drop.setNodeRef(node); }}
    disabled={disabled}
    style={{ ...style, opacity: drag.isDragging ? 0.45 : undefined }}
    className={cn(
      "cursor-grab active:cursor-grabbing",
      drop.isOver && "ring-1 ring-primary",
      className,
    )}
    {...props}
    {...drag.attributes}
    {...drag.listeners}
  />;
}
/* eslint-enable react-hooks/refs */

export interface ServerTabActions {
  selectedHostId?: string;
  onSelectHost: (hostId: string) => void;
  onSelectTeam: (hostId: string, team: string) => void;
  onCreate: (hostId: string) => void;
  onEditServer: (hostId: string) => void;
  onRemoveServer: (hostId: string) => void;
  onConnectHost: (hostId: string) => void;
}

/**
 * The machines, as they are: every server with its agents, its groups and the
 * order the operator dragged them into. The one tab that persists an order, so
 * the one tab whose rows are drag sources.
 */
export function ServersTab({ hosts, filtering, daemonViews, appVersion, actions, servers }: {
  hosts: HostAgents[];
  /** A search is on, so a group with no surviving member is hidden. */
  filtering: boolean;
  daemonViews: DaemonMeta[];
  appVersion: string;
  actions: AgentRowActions;
  servers: ServerTabActions;
}) {
  const viewFor = (hostId: string) => daemonViews.find((candidate) => candidate.id === hostId);
  const pendingUpdates = hosts.filter((host) => {
    const view = viewFor(host.host.id);
    return view ? hostUpdateAvailable(view, appVersion) : false;
  });

  return (
    <>
      {pendingUpdates.length > 0 && (
        <div className="flex justify-end px-2 pt-1.5">
          <Button
            variant="outline"
            size="sm"
            className="h-5 rounded-[6px] border-border px-[7px] text-[10.5px] font-medium tracking-[.02em] text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
            title="Update all servers with available updates"
            onClick={() => { for (const host of pendingUpdates) servers.onEditServer(host.host.id); }}
          >
            Update all ({pendingUpdates.length})
          </Button>
        </div>
      )}
      {hosts.map((host) => {
        const view = viewFor(host.host.id);
        const agents = hostAgents(host);
        const groupNames = [...new Set([
          ...(host.groups ?? []).map((group) => group.name),
          ...agents.map((row) => row.group).filter(Boolean),
        ])];
        const teams = new Map<string, typeof agents>();
        for (const name of ordered(groupNames, host.sidebarOrder?.groups ?? [], (name) => name)) {
          teams.set(name, []);
        }
        for (const row of agents) {
          if (!row.group) continue;
          teams.set(row.group, [...(teams.get(row.group) ?? []), row]);
        }
        const individuals = agents.filter((row) => !row.group);
        const disabled = Boolean(host.error);

        return (
          <section key={host.host.id || "__local__"} className="pb-2">
            <SidebarSectionHeader
              title={
                <SortableButton
                  dragKey={dragId("servers", "", host.host.id, undefined, host.host.label)}
                  type="button"
                  aria-label={`Open server ${host.host.label}`}
                  aria-current={servers.selectedHostId === host.host.id ? "page" : undefined}
                  onClick={() => servers.onSelectHost(host.host.id)}
                  className={cn(
                    SECTION_TITLE_CLASS,
                    "px-1 py-0.5 hover:bg-sidebar-accent",
                    servers.selectedHostId === host.host.id && "bg-sidebar-accent text-foreground",
                  )}
                  title={host.host.label}
                >
                  {host.host.label}
                </SortableButton>
              }
              state={host.error ? "offline" : view?.state ?? "ready"}
              update={view && hostUpdateAvailable(view, appVersion)
                ? () => servers.onEditServer(host.host.id)
                : undefined}
              updateLabel={`Update ${host.host.label}`}
              menu={
                // The implicit local host has no registry entry (id ""), so
                // there is nothing to edit or remove for it.
                host.host.id !== "" && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        className={SECTION_ICON_CLASS}
                        aria-label={`manage ${host.host.label}`}
                      >
                        ⋯
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => servers.onEditServer(host.host.id)}>Edit host</DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => servers.onRemoveServer(host.host.id)}>Remove host</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )
              }
              onAdd={disabled ? undefined : () => servers.onCreate(host.host.id)}
              addLabel={`new agent on ${host.host.label}`}
            />
            {view?.kind === "ssh" && (
              <div className="px-2 pb-1">
                <HostStatus
                  host={view}
                  appVersion={appVersion}
                  showUpdate={false}
                  showState={false}
                  onConnect={() => servers.onConnectHost(view.id)}
                  onUpdate={() => servers.onEditServer(view.id)}
                />
              </div>
            )}
            {host.error && <div className="px-2 text-xs text-destructive">{host.error}</div>}
            {teams.size > 0 && (
              <div className="mt-1">
                <div className="px-2 pt-1.5 pb-1 text-[11px] font-medium tracking-[.02em] text-muted-foreground">Teams</div>
                {[...teams.entries()]
                  .filter(([, rows]) => !filtering || rows.length > 0)
                  .map(([name, rows]) => (
                    <details key={name} open>
                      <summary className="cursor-pointer px-2 py-1 text-sm font-medium">
                        <SortableButton
                          dragKey={dragId("groups", host.host.id, name)}
                          type="button"
                          aria-label={`Open team ${name}`}
                          className="hover:underline"
                          disabled={disabled}
                          onClick={(event) => {
                            event.preventDefault();
                            event.stopPropagation();
                            if (!disabled) servers.onSelectTeam(host.host.id, name);
                          }}
                        >{name}</SortableButton>
                      </summary>
                      <div className="pl-2">
                        {rows.map((row) => (
                          <SidebarAgentRow
                            key={row.key}
                            row={row}
                            actions={actions}
                            dragKey={dragId("agents", host.host.id, row.agent.name, row.group)}
                          />
                        ))}
                      </div>
                    </details>
                  ))}
              </div>
            )}
            <div className="mt-1">
              <div className="px-2 pt-1.5 pb-1 text-[11px] font-medium tracking-[.02em] text-muted-foreground">Individual agents</div>
              {individuals.map((row) => (
                <SidebarAgentRow
                  key={row.key}
                  row={row}
                  actions={actions}
                  dragKey={dragId("agents", host.host.id, row.agent.name, "")}
                />
              ))}
            </div>
            {!host.error && host.agents.length === 0 && <SectionEmpty />}
          </section>
        );
      })}
    </>
  );
}
