import { useRef, useState, type ButtonHTMLAttributes } from "react";
import { Plus, Search, Server } from "lucide-react";
import {
  DndContext, KeyboardSensor, PointerSensor, useDraggable, useDroppable,
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
import {
  dragId, identityFor, rowCollision, sameScope, sidebarAnnouncements, type ReorderKind,
} from "./sidebarDnd";

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
  const [query, setQuery] = useState("");
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
    const source = identityFor(active.id);
    const target = identityFor(over.id);
    if (!sameScope(source, target)) return;
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

  // The reference opens a Command palette from this box; this app has no
  // palette yet, and a dead control is worse than a live one — so it filters
  // the list the sidebar already has, by agent name or server label.
  const q = query.trim().toLowerCase();
  const filtering = q.length > 0;
  const visibleHosts = filtering
    ? hosts
      .map((h) => ({ ...h, agents: h.agents.filter((a) => a.name.toLowerCase().includes(q)) }))
      .filter((h) => h.agents.length > 0 || h.host.label.toLowerCase().includes(q))
    : hosts;
  const connectedCount = hosts.filter((h) => !h.error).length;
  const agentCount = hosts.reduce((total, h) => total + h.agents.length, 0);

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={rowCollision}
      accessibility={{ announcements: sidebarAnnouncements }}
      onDragEnd={finishReorder}
    >
    {/* The sidebar is chrome, not a panel: it sits straight on --background
        with no border and no surface of its own — the content island is the
        only raised thing on the screen. */}
    <aside
      ref={asideRef}
      aria-label="Agents"
      data-testid="agents-sidebar"
      style={{ width }}
      className="flex min-h-0 shrink-0 flex-col px-0.5"
    >
      <div className="shrink-0 px-1.5 pt-0.5 pb-2">
        <label className="flex h-[30px] items-center gap-[7px] rounded-[8px] bg-muted px-[9px] text-muted-foreground">
          <Search className="size-[13px] shrink-0" aria-hidden="true" />
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label="Search agents"
            placeholder="Search agents"
            className="min-w-0 flex-1 bg-transparent text-[12.5px] text-foreground outline-none placeholder:text-muted-foreground"
          />
        </label>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
      {visibleHosts.map((h) => (
        <section key={h.host.id || "__local__"} className="px-1.5 pb-2">
          <div className="flex items-center justify-between">
            <SortableButton
              dragKey={dragId("servers", "", h.host.id, undefined, h.host.label)}
              type="button"
              aria-label={`Open server ${h.host.label}`}
              aria-current={selectedHostId === h.host.id ? "page" : undefined}
              onClick={() => onSelectHost(h.host.id)}
              className={cn(
                "min-w-0 truncate rounded-[7px] px-2 py-1 text-left text-[11px] font-medium tracking-[.02em] uppercase text-muted-foreground hover:bg-sidebar-accent hover:text-foreground",
                selectedHostId === h.host.id && "bg-sidebar-accent text-foreground",
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
                    <div className="px-2 pt-1.5 pb-1 text-[11px] font-medium tracking-[.02em] text-muted-foreground">Teams</div>
                    {[...teams.entries()]
                      .filter(([, agents]) => !filtering || agents.length > 0)
                      .map(([name, agents]) => (
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
                  <div className="px-2 pt-1.5 pb-1 text-[11px] font-medium tracking-[.02em] text-muted-foreground">Individual agents</div>
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
      </div>
      <div className="flex h-11 shrink-0 items-center gap-2 px-3">
        <span aria-hidden="true" className="grid size-[22px] shrink-0 place-items-center rounded-full bg-accent text-muted-foreground">
          <Server className="size-3" />
        </span>
        <span className="min-w-0 flex-1 text-[12px] leading-tight">
          <span className="block truncate font-medium">All servers</span>
          <span className="block text-[11px] text-muted-foreground">
            {connectedCount} connected · {agentCount} agents
          </span>
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="size-6 rounded-[7px] text-muted-foreground hover:bg-accent hover:text-foreground"
          aria-label="Add host"
          title="Add host"
          onClick={onAddServer}
        >
          <Plus className="size-3" />
        </Button>
        <span
          role="img"
          aria-label={`${connectedCount} of ${hosts.length} servers connected`}
          title={`${connectedCount} of ${hosts.length} servers connected`}
          className={cn(
            "size-1.5 shrink-0 rounded-full",
            connectedCount === hosts.length ? "bg-status-running" : "bg-status-failed",
          )}
        />
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
      className="w-2 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-primary/30 focus-visible:bg-primary/30 focus-visible:outline-none"
    />
    </DndContext>
  );
}
