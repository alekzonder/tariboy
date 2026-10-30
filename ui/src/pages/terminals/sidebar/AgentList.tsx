import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { HostStatus } from "@/components/HostStatus";
import { cn } from "@/lib/utils";
import type { HostAgents } from "@/lib/aggregate";
import { hostUpdateAvailable, type DaemonMeta } from "@/lib/daemons";
import { dragId } from "./sidebarDnd";
import {
  SECTION_ICON_CLASS,
  SECTION_TITLE_CLASS,
  SectionEmpty,
  SidebarSectionHeader,
} from "./SidebarSectionHeader";
import { SidebarAgentRow, type AgentRowActions } from "./SidebarAgentRow";
import type { HostSection, SidebarAgent, sidebarLayout } from "./sidebarModel";
import { SortableButton } from "./SortableButton";

export interface HostActions {
  selectedHostId?: string;
  onSelectHost: (hostId: string) => void;
  onSelectTeam: (hostId: string, team: string) => void;
  onEditServer: (hostId: string) => void;
  onRemoveServer: (hostId: string) => void;
  onConnectHost: (hostId: string) => void;
}

const SUBHEADING_CLASS =
  "px-2 pt-1.5 pb-1 text-caption font-medium tracking-[.02em] text-muted-foreground";

/** The host's own menu. The implicit local host has no registry entry (id
 *  ""), so there is nothing to edit or remove for it. */
function HostMenu({ host, actions }: { host: HostAgents; actions: HostActions }) {
  if (host.host.id === "") return null;
  return (
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
        <DropdownMenuItem onSelect={() => actions.onEditServer(host.host.id)}>
          Edit host
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => actions.onRemoveServer(host.host.id)}>
          Remove host
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function HostHeader({
  host,
  view,
  appVersion,
  actions,
}: {
  host: HostAgents;
  view?: DaemonMeta;
  appVersion: string;
  actions: HostActions;
}) {
  const selected = actions.selectedHostId === host.host.id;
  return (
    <SidebarSectionHeader
      title={
        <SortableButton
          dragKey={dragId("servers", "", host.host.id, undefined, host.host.label)}
          type="button"
          aria-label={`Open host ${host.host.label}`}
          aria-current={selected ? "page" : undefined}
          onClick={() => actions.onSelectHost(host.host.id)}
          className={cn(
            SECTION_TITLE_CLASS,
            "px-1 py-0.5 hover:bg-sidebar-accent",
            selected && "bg-sidebar-accent text-foreground",
          )}
          title={host.host.label}
        >
          {host.host.label}
        </SortableButton>
      }
      state={host.error ? "offline" : (view?.state ?? "ready")}
      update={
        view && hostUpdateAvailable(view, appVersion)
          ? () => actions.onEditServer(host.host.id)
          : undefined
      }
      updateLabel={`Update ${host.host.label}`}
      menu={<HostMenu host={host} actions={actions} />}
    />
  );
}

function TeamsSection({
  hostId,
  teams,
  filtering,
  disabled,
  rowActions,
  onSelectTeam,
}: {
  hostId: string;
  teams: Map<string, SidebarAgent[]>;
  filtering: boolean;
  disabled: boolean;
  rowActions: AgentRowActions;
  onSelectTeam: (hostId: string, team: string) => void;
}) {
  if (teams.size === 0) return null;
  return (
    <div className="mt-1">
      <div className={SUBHEADING_CLASS}>Teams</div>
      {[...teams.entries()]
        .filter(([, rows]) => !filtering || rows.length > 0)
        .map(([name, rows]) => (
          <details key={name} open>
            <summary className="cursor-pointer px-2 py-1 text-sm font-medium">
              <SortableButton
                dragKey={dragId("groups", hostId, name)}
                type="button"
                aria-label={`Open team ${name}`}
                className="hover:underline"
                disabled={disabled}
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  if (!disabled) onSelectTeam(hostId, name);
                }}
              >
                {name}
              </SortableButton>
            </summary>
            <div className="pl-2">
              {rows.map((row) => (
                <SidebarAgentRow
                  key={row.key}
                  row={row}
                  actions={rowActions}
                  dragKey={dragId("agents", hostId, row.agent.name, row.group)}
                />
              ))}
            </div>
          </details>
        ))}
    </div>
  );
}

function HostPart({
  section,
  byHost,
  view,
  filtering,
  appVersion,
  rowActions,
  actions,
}: {
  section: HostSection;
  byHost: boolean;
  view?: DaemonMeta;
  filtering: boolean;
  appVersion: string;
  rowActions: AgentRowActions;
  actions: HostActions;
}) {
  const { host, teams, individuals } = section;
  return (
    <section className="pb-2">
      {byHost && <HostHeader host={host} view={view} appVersion={appVersion} actions={actions} />}
      {byHost && view?.kind === "ssh" && (
        <div className="px-2 pb-1">
          <HostStatus
            host={view}
            appVersion={appVersion}
            showUpdate={false}
            showState={false}
            onConnect={() => actions.onConnectHost(view.id)}
            onUpdate={() => actions.onEditServer(view.id)}
          />
        </div>
      )}
      {host.error && <div className="px-2 text-xs text-destructive">{host.error}</div>}
      <TeamsSection
        hostId={host.host.id}
        teams={teams}
        filtering={filtering}
        disabled={Boolean(host.error)}
        rowActions={rowActions}
        onSelectTeam={actions.onSelectTeam}
      />
      {/* The heading only separates the agents from teams above them. */}
      {teams.size > 0 && <div className={cn(SUBHEADING_CLASS, "mt-1")}>Individual agents</div>}
      {individuals.map((row) => (
        <SidebarAgentRow
          key={row.key}
          row={row}
          actions={rowActions}
          dragKey={dragId("agents", host.host.id, row.agent.name, "")}
        />
      ))}
      {byHost && !host.error && host.agents.length === 0 && <SectionEmpty />}
    </section>
  );
}

/**
 * The agent list under Agents: pinned agents on top, then teams and agents in
 * the order the operator dragged them into. With two or more hosts each host
 * gets its heading, state and menu; with one the list is flat.
 */
export function AgentList({
  layout,
  filtering,
  daemonViews,
  appVersion,
  rowActions,
  actions,
}: {
  layout: ReturnType<typeof sidebarLayout>;
  /** A search is on, so a team with no surviving member is hidden. */
  filtering: boolean;
  daemonViews: DaemonMeta[];
  appVersion: string;
  rowActions: AgentRowActions;
  actions: HostActions;
}) {
  const viewFor = (hostId: string) => daemonViews.find((candidate) => candidate.id === hostId);
  const empty =
    layout.pinned.length === 0 &&
    layout.sections.every(
      (section) =>
        section.host.agents.length === 0 && section.teams.size === 0 && !section.host.error,
    );
  if (empty && !layout.byHost) {
    return <div className="px-2 pt-1.5 text-body-sm text-muted-foreground">No agents.</div>;
  }
  return (
    <>
      {layout.pinned.map((row) => (
        <SidebarAgentRow key={row.key} row={row} actions={rowActions} />
      ))}
      {layout.pinned.length > 0 && (
        <div
          aria-hidden="true"
          className="mx-3.5 my-2.5 h-px bg-[color-mix(in_oklab,var(--border)_55%,transparent)]"
        />
      )}
      {layout.sections.map((section) => (
        <HostPart
          key={section.host.host.id || "__local__"}
          section={section}
          byHost={layout.byHost}
          view={viewFor(section.host.host.id)}
          filtering={filtering}
          appVersion={appVersion}
          rowActions={rowActions}
          actions={actions}
        />
      ))}
    </>
  );
}
