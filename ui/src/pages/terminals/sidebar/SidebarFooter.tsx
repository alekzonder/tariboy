import { Link } from "react-router-dom";
import { Server } from "lucide-react";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { cn } from "@/lib/utils";

/**
 * The foot of the sidebar: one line that says how the workspace's hosts are
 * and opens Settings → Hosts, with a dot when a host has an update. Adding,
 * updating and grouping hosts all happen on that page.
 */
export function SidebarFooter({
  line,
  healthy,
  behind,
  onNavigate,
}: {
  line: string;
  healthy: boolean;
  /** The hosts that run an older daemon than this app. */
  behind: string[];
  onNavigate?: () => void;
}) {
  const update =
    behind.length > 0
      ? `${behind.length} host${behind.length === 1 ? "" : "s"} can be updated`
      : "";
  return (
    <div className="flex h-11 shrink-0 items-center gap-1 px-1.5">
      <Link
        to="/settings/hosts"
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
    </div>
  );
}
