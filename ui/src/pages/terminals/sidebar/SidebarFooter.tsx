import { useRef } from "react";
import { Link } from "react-router-dom";
import { Check, MoreHorizontal, Server } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { selectWorkspace, useWorkspaces } from "@/lib/workspaces";
import { cn } from "@/lib/utils";

/**
 * The foot of the sidebar: one line that says how the workspace's hosts are
 * and opens Settings → Hosts, a dot when a host has an update, and a menu to
 * add a host, update the hosts that are behind, and switch or manage host
 * workspaces.
 */
export function SidebarFooter({
  line,
  healthy,
  hostsPath,
  behind,
  onAddServer,
  onUpdateAll,
  onSelectWorkspace,
  onManageWorkspaces,
  onNavigate,
}: {
  line: string;
  healthy: boolean;
  /** Settings → Hosts of the page host. */
  hostsPath: string;
  /** The hosts that run an older daemon than this app. */
  behind: string[];
  onAddServer: () => void;
  onUpdateAll: (hostIds: string[]) => void;
  onSelectWorkspace: (id: string) => void;
  /** Opens the workspace manager; `focusNew` for "New workspace…". */
  onManageWorkspaces: (focusNew: boolean) => void;
  onNavigate?: () => void;
}) {
  const workspaces = useWorkspaces();
  // The manager dialog takes focus; handing it back to the trigger as the
  // menu closes would steal it.
  const managing = useRef(false);
  const manage = (focusNew: boolean) => {
    managing.current = true;
    onManageWorkspaces(focusNew);
  };
  const update =
    behind.length > 0
      ? `${behind.length} host${behind.length === 1 ? "" : "s"} can be updated`
      : "";
  return (
    <div className="flex h-11 shrink-0 items-center gap-1 px-1.5">
      <Link
        to={hostsPath}
        onClick={onNavigate}
        data-tour={TOUR_ANCHORS.hostsLine}
        title="Hosts"
        className="flex h-7.5 min-w-0 flex-1 items-center gap-2 rounded-control px-1.5 hover:bg-sidebar-accent"
      >
        <span
          aria-hidden="true"
          className="grid size-5.5 shrink-0 place-items-center rounded-full bg-accent text-muted-foreground"
        >
          <Server className="size-3" />
        </span>
        <span className="min-w-0 truncate text-ui text-muted-foreground">{line}</span>
        <span
          aria-hidden="true"
          className={cn(
            "size-1.5 shrink-0 rounded-full",
            healthy ? "bg-status-running" : "bg-status-failed",
          )}
        />
        {update && (
          <span
            role="img"
            aria-label={update}
            title={update}
            className="size-1.5 shrink-0 rounded-full bg-primary"
          />
        )}
      </Link>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon-xs"
            className="size-6 shrink-0 rounded-control text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
            aria-label="Hosts menu"
            title="Hosts menu"
            data-tour={TOUR_ANCHORS.addHost}
          >
            <MoreHorizontal className="size-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          side="top"
          className="w-60"
          onCloseAutoFocus={(event) => {
            if (managing.current) event.preventDefault();
            managing.current = false;
          }}
        >
          <DropdownMenuItem onSelect={onAddServer}>Add host…</DropdownMenuItem>
          {behind.length > 0 && (
            <DropdownMenuItem onSelect={() => onUpdateAll(behind)}>
              Update all ({behind.length})
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          {workspaces.workspaces.length > 1 && (
            <>
              <DropdownMenuLabel className="text-caption font-semibold tracking-[.06em] uppercase">
                Workspaces
              </DropdownMenuLabel>
              {workspaces.workspaces.map((workspace) => (
                <DropdownMenuItem
                  key={workspace.id}
                  role="menuitemradio"
                  aria-checked={workspace.id === workspaces.active}
                  onSelect={() => {
                    selectWorkspace(workspace.id);
                    onSelectWorkspace(workspace.id);
                  }}
                >
                  <span className="min-w-0 flex-1 truncate">{workspace.name}</span>
                  {workspace.id === workspaces.active && (
                    <Check aria-hidden="true" className="size-3 text-primary" />
                  )}
                </DropdownMenuItem>
              ))}
            </>
          )}
          <DropdownMenuItem onSelect={() => manage(true)}>New workspace…</DropdownMenuItem>
          <DropdownMenuItem onSelect={() => manage(false)}>Manage workspaces…</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
