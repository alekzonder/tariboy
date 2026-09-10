import { useRef, type ButtonHTMLAttributes } from "react";
import {
  DndContext, KeyboardSensor, PointerSensor, closestCenter, pointerWithin, useDraggable, useDroppable,
  useSensor, useSensors, type DragEndEvent,
} from "@dnd-kit/core";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { cn } from "@/lib/utils";
import type { HostAgents } from "@/lib/aggregate";
import type { DaemonMeta } from "@/lib/daemons";
import { HostStatus } from "@/components/HostStatus";
import {
  DEFAULT_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH, MIN_SIDEBAR_WIDTH,
} from "./useSidebarWidth";

function ordered<T>(items: T[], ids: string[], id: (item: T) => string): T[] {
  const rank = new Map(ids.map((value, index) => [value, index]));
  return items
    .map((item, index) => ({ item, index, rank: rank.get(id(item)) }))
    .sort((left, right) => (left.rank ?? ids.length + left.index) - (right.rank ?? ids.length + right.index))
    .map(({ item }) => item);
}

function move(ids: string[], active: string, over: string): string[] {
  const from = ids.indexOf(active);
  const to = ids.indexOf(over);
  if (from < 0 || to < 0 || from === to) return ids;
  const next = [...ids];
  next.splice(to, 0, next.splice(from, 1)[0]);
  return next;
}

type ReorderKind = "servers" | "groups" | "agents";
type DragIdentity = [ReorderKind, string, string, string?];
const dragId = (...identity: DragIdentity) => JSON.stringify(identity);
const rowCollision: typeof closestCenter = (args) => {
  const pointer = pointerWithin(args);
  return pointer.length ? pointer : closestCenter(args);
};

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
    className={cn("cursor-grab active:cursor-grabbing", className)}
    {...props}
    {...drag.attributes}
    {...drag.listeners}
  />;
}

export function TerminalsSidebar({ hosts, selectedHostId, selected, onSelectHost, onSelect, onSelectTeam, onReorder, onClone, onCreate, onAddServer, onEditServer, onRemoveServer, daemonViews, appVersion, onConnectHost, attention, width, onResize }: {
  hosts: HostAgents[];
  selectedHostId?: string;
  selected?: { hostId: string; agent: string };
  onSelectHost: (hostId: string) => void;
  onSelect: (hostId: string, agent: string) => void;
  onSelectTeam: (hostId: string, team: string) => void;
  onReorder: (kind: ReorderKind, hostId: string, ids: string[]) => void;
  onClone: (hostId: string, agentName: string) => void;
  onCreate: (hostId: string) => void;
  onAddServer: () => void;
  onEditServer: (hostId: string) => void;
  onRemoveServer: (hostId: string) => void;
  daemonViews: DaemonMeta[];
  appVersion: string;
  onConnectHost: (hostId: string) => void;
  attention: ReadonlySet<string>;
  width: number;
  onResize: (px: number) => void;
}) {
  const asideRef = useRef<HTMLElement | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor),
  );

  // Drag state lives on window listeners rather than pointer capture: capture
  // is spotty in jsdom, and window listeners keep tracking the drag when the
  // pointer leaves the 4px handle (which it always does).
  const startDrag = (e: React.PointerEvent) => {
    e.preventDefault();
    const left = asideRef.current?.getBoundingClientRect().left ?? 0;
    const onMove = (ev: PointerEvent) => onResize(ev.clientX - left);
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      document.body.style.removeProperty("cursor");
      document.body.style.removeProperty("user-select");
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    // Keep the resize cursor (and kill text selection) for the whole drag, not
    // just while the pointer is over the handle.
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const step = e.shiftKey ? 32 : 8;
    if (e.key === "ArrowLeft") { e.preventDefault(); onResize(width - step); }
    else if (e.key === "ArrowRight") { e.preventDefault(); onResize(width + step); }
    else if (e.key === "Home") { e.preventDefault(); onResize(DEFAULT_SIDEBAR_WIDTH); }
  };

  const finishReorder = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    const source = JSON.parse(String(active.id)) as DragIdentity;
    const target = JSON.parse(String(over.id)) as DragIdentity;
    if (source[0] !== target[0] || source[1] !== target[1] || source[3] !== target[3]) return;
    if (source[0] === "servers") {
      onReorder("servers", "", move(hosts.map((host) => host.host.id), source[2], target[2]));
      return;
    }
    const host = hosts.find((entry) => entry.host.id === source[1]);
    if (!host) return;
    if (source[0] === "groups") {
      const names = [...new Set([
        ...(host.groups ?? []).map((group) => group.name),
        ...host.agents.map((agent) => agent.group?.trim()).filter((name): name is string => Boolean(name)),
      ])];
      const current = ordered(names, host.sidebarOrder?.groups ?? [], (name) => name);
      onReorder("groups", source[1], move(current, source[2], target[2]));
      return;
    }
    const current = ordered(host.agents, host.sidebarOrder?.agents ?? [], (agent) => agent.name)
      .map((agent) => agent.name);
    onReorder("agents", source[1], move(current, source[2], target[2]));
  };

  return (
    <DndContext sensors={sensors} collisionDetection={rowCollision} onDragEnd={finishReorder}>
    <aside ref={asideRef} style={{ width }} className="flex shrink-0 flex-col overflow-y-auto border-r">
      <div className="p-2 text-sm font-semibold">Agents</div>
      {hosts.map((h) => (
        <section key={h.host.id || "__local__"} className="px-2 pb-2">
          <div className="flex items-center justify-between">
            <SortableButton
              dragKey={dragId("servers", "", h.host.id)}
              type="button"
              aria-label={`Open server ${h.host.label}`}
              aria-current={selectedHostId === h.host.id ? "page" : undefined}
              onClick={() => onSelectHost(h.host.id)}
              className={cn(
                "min-w-0 truncate rounded px-2 py-1 text-left text-xs font-semibold uppercase text-muted-foreground hover:bg-accent hover:text-foreground",
                selectedHostId === h.host.id && "bg-accent text-foreground",
              )}
              title={h.host.label}
            >
              {h.host.label}
            </SortableButton>
            <span className="flex shrink-0 items-center">
              {/* The implicit local host has no registry entry (id ""), so there
                  is nothing to edit or remove for it. */}
              {h.host.id !== "" && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="sm" aria-label={`manage ${h.host.label}`}>⋯</Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onSelect={() => onEditServer(h.host.id)}>Edit host</DropdownMenuItem>
                    <DropdownMenuItem onSelect={() => onRemoveServer(h.host.id)}>Remove host</DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
              <Button variant="ghost" size="sm" aria-label={`new agent on ${h.host.label}`}
                disabled={Boolean(h.error)}
                onClick={() => onCreate(h.host.id)}>+</Button>
            </span>
          </div>
          {h.host.id !== "" && (() => {
            const view = daemonViews.find((candidate) => candidate.id === h.host.id);
            return view?.kind === "ssh" ? (
              <HostStatus
                host={view}
                appVersion={appVersion}
                onConnect={() => onConnectHost(view.id)}
                onUpdate={() => onEditServer(view.id)}
              />
            ) : null;
          })()}
          {h.error && <div className="text-xs text-destructive">{h.error}</div>}
          {(() => {
            const agentsInOrder = ordered(h.agents, h.sidebarOrder?.agents ?? [], (agent) => agent.name);
            const renderAgent = (a: (typeof h.agents)[number]) => {
            const interactive = a.interactive !== false;
            return (
              <ContextMenu key={a.name}>
                <ContextMenuTrigger asChild>
                  <div
                    className={cn(
                      "flex w-full items-center rounded text-sm hover:bg-accent",
                      selected && selected.hostId === h.host.id && selected.agent === a.name && "bg-accent",
                    )}
                  >
                    <SortableButton
                      dragKey={dragId("agents", h.host.id, a.name, a.group?.trim() ?? "")}
                      type="button"
                      className="flex min-w-0 flex-1 items-center justify-between px-2 py-1 text-left"
                      aria-label={`Open ${a.name}`}
                      aria-current={selected?.hostId === h.host.id && selected.agent === a.name ? "page" : undefined}
                      disabled={Boolean(h.error)}
                      onClick={() => { if (!h.error) onSelect(h.host.id, a.name); }}
                    >
                      <span className="flex min-w-0 items-center gap-1">
                        <span className="truncate">{a.name}</span>
                        {attention.has(JSON.stringify([h.host.id, a.name])) && (
                          <span
                            role="img"
                            aria-label={`Unread customer question for ${a.name} on ${h.host.label}`}
                            title={`Unread customer question for ${a.name} on ${h.host.label}`}
                            className="h-2 w-2 shrink-0 rounded-full bg-red-500"
                          />
                        )}
                        {!interactive && (
                          <span className="shrink-0 text-xs text-muted-foreground" title="not interactive (no tty)">
                            non-tty
                          </span>
                        )}
                      </span>
						{a.budget?.exhausted?.length ? <Badge variant="destructive" title={`Out of budget: ${a.budget.exhausted.join(", ")}`}>out of budget</Badge> : <Badge variant={a.state === "running" ? "default" : "secondary"}>{a.state}</Badge>}
                    </SortableButton>
                  </div>
                </ContextMenuTrigger>
                <ContextMenuContent>
                  <ContextMenuItem disabled={Boolean(h.error)} onSelect={() => onClone(h.host.id, a.name)}>
                    Clone
                  </ContextMenuItem>
                </ContextMenuContent>
              </ContextMenu>
            );
            };
            const groupNames = [...new Set([
              ...(h.groups ?? []).map((group) => group.name),
              ...agentsInOrder.map((agent) => agent.group?.trim()).filter((name): name is string => Boolean(name)),
            ])];
            const teams = new Map<string, typeof h.agents>();
            for (const name of ordered(groupNames, h.sidebarOrder?.groups ?? [], (name) => name)) teams.set(name, []);
            for (const agent of agentsInOrder) {
              const group = agent.group?.trim();
              if (!group) continue;
              teams.set(group, [...(teams.get(group) ?? []), agent]);
            }
            const individuals = agentsInOrder.filter((agent) => !agent.group?.trim());

            return (
              <>
                {teams.size > 0 && (
                  <div className="mt-1">
                    <div className="px-2 py-1 text-xs font-semibold text-muted-foreground">Teams</div>
                    {[...teams.entries()].map(([name, agents]) => (
                      <details key={name} open>
                        <summary className="cursor-pointer px-2 py-1 text-sm font-medium">
                          <SortableButton
                            dragKey={dragId("groups", h.host.id, name)}
                            type="button"
                            aria-label={`Open team ${name}`}
                            className="hover:underline"
                            disabled={Boolean(h.error)}
                            onClick={(event) => { event.preventDefault(); event.stopPropagation(); if (!h.error) onSelectTeam(h.host.id, name); }}
                          >{name}</SortableButton>
                        </summary>
                        <div className="pl-2">{agents.map(renderAgent)}</div>
                      </details>
                    ))}
                  </div>
                )}
                <div className="mt-1">
                  <div className="px-2 py-1 text-xs font-semibold text-muted-foreground">Individual agents</div>
                  {individuals.map(renderAgent)}
                </div>
              </>
            );
          })()}
          {!h.error && h.agents.length === 0 && (
            <div className="px-2 text-xs text-muted-foreground">No agents.</div>
          )}
        </section>
      ))}
      <div className="mt-auto p-2">
        <Button variant="outline" size="sm" className="w-full" onClick={onAddServer}>Add host</Button>
      </div>
    </aside>
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="resize sidebar"
      aria-valuenow={width}
      aria-valuemin={MIN_SIDEBAR_WIDTH}
      aria-valuemax={MAX_SIDEBAR_WIDTH}
      tabIndex={0}
      onPointerDown={startDrag}
      onKeyDown={onKeyDown}
      onDoubleClick={() => onResize(DEFAULT_SIDEBAR_WIDTH)}
      className="w-1 shrink-0 cursor-col-resize bg-border transition-colors hover:bg-primary focus-visible:bg-primary focus-visible:outline-none"
    />
    </DndContext>
  );
}
