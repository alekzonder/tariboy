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
import { groupTeams, hostAgents, type SidebarAgent } from "./sidebarModel";
import { SortableButton } from "./SortableButton";

export interface ServerTabActions {
  selectedHostId?: string;
  onSelectHost: (hostId: string) => void;
  onSelectTeam: (hostId: string, team: string) => void;
  onCreate: (hostId: string) => void;
  onEditServer: (hostId: string) => void;
  onUpdateAll: (hostIds: string[]) => void;
  onRemoveServer: (hostId: string) => void;
  onConnectHost: (hostId: string) => void;
}

const SUBHEADING_CLASS =
  "px-2 pt-1.5 pb-1 text-caption font-medium tracking-[.02em] text-muted-foreground";

/** The server's own menu. The implicit local host has no registry entry (id
 *  ""), so there is nothing to edit or remove for it. */
function ServerMenu({ host, servers }: { host: HostAgents; servers: ServerTabActions }) {
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
        <DropdownMenuItem onSelect={() => servers.onEditServer(host.host.id)}>
          Edit host
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => servers.onRemoveServer(host.host.id)}>
          Remove host
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ServerSectionHeader({
  host,
  view,
  appVersion,
  servers,
}: {
  host: HostAgents;
  view?: DaemonMeta;
  appVersion: string;
  servers: ServerTabActions;
}) {
  const selected = servers.selectedHostId === host.host.id;
  return (
    <SidebarSectionHeader
      title={
        <SortableButton
          dragKey={dragId("servers", "", host.host.id, undefined, host.host.label)}
          type="button"
          aria-label={`Open server ${host.host.label}`}
          aria-current={selected ? "page" : undefined}
          onClick={() => servers.onSelectHost(host.host.id)}
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
          ? () => servers.onEditServer(host.host.id)
          : undefined
      }
      updateLabel={`Update ${host.host.label}`}
      menu={<ServerMenu host={host} servers={servers} />}
      onAdd={host.error ? undefined : () => servers.onCreate(host.host.id)}
      addLabel={`new agent on ${host.host.label}`}
    />
  );
}

function TeamsSection({
  hostId,
  teams,
  filtering,
  disabled,
  actions,
  onSelectTeam,
}: {
  hostId: string;
  teams: Map<string, SidebarAgent[]>;
  filtering: boolean;
  disabled: boolean;
  actions: AgentRowActions;
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
                  actions={actions}
                  dragKey={dragId("agents", hostId, row.agent.name, row.group)}
                />
              ))}
            </div>
          </details>
        ))}
    </div>
  );
}

function ServerSection({
  host,
  view,
  filtering,
  appVersion,
  actions,
  servers,
}: {
  host: HostAgents;
  view?: DaemonMeta;
  filtering: boolean;
  appVersion: string;
  actions: AgentRowActions;
  servers: ServerTabActions;
}) {
  const agents = hostAgents(host);
  const individuals = agents.filter((row) => !row.group);
  return (
    <section className="pb-2">
      <ServerSectionHeader host={host} view={view} appVersion={appVersion} servers={servers} />
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
      <TeamsSection
        hostId={host.host.id}
        teams={groupTeams(host, agents)}
        filtering={filtering}
        disabled={Boolean(host.error)}
        actions={actions}
        onSelectTeam={servers.onSelectTeam}
      />
      <div className="mt-1">
        <div className={SUBHEADING_CLASS}>Individual agents</div>
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
}

/**
 * The machines, as they are: every server with its agents, its groups and the
 * order the operator dragged them into. The one tab that persists an order, so
 * the one tab whose rows are drag sources.
 */
export function ServersTab({
  hosts,
  filtering,
  daemonViews,
  appVersion,
  actions,
  servers,
}: {
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
            className="h-5 rounded-sm border-border px-1.75 text-tiny font-medium tracking-[.02em] text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
            title="Update all servers with available updates"
            onClick={() => servers.onUpdateAll(pendingUpdates.map((host) => host.host.id))}
          >
            Update all ({pendingUpdates.length})
          </Button>
        </div>
      )}
      {hosts.map((host) => (
        <ServerSection
          key={host.host.id || "__local__"}
          host={host}
          view={viewFor(host.host.id)}
          filtering={filtering}
          appVersion={appVersion}
          actions={actions}
          servers={servers}
        />
      ))}
    </>
  );
}
