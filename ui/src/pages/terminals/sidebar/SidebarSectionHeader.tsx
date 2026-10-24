import type { ReactNode } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** The name of a group or a server, wherever it is clickable. */
export const SECTION_TITLE_CLASS =
  "min-w-0 truncate rounded-sm text-left text-caption font-semibold tracking-[.06em] uppercase text-muted-foreground hover:text-foreground";

/** Icon buttons in a section header are 20px, not the 24px of the tab row. */
export const SECTION_ICON_CLASS =
  "size-5 shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-accent hover:text-foreground";

/** A server's state, said in the header rather than in a separate row. Only
 *  `ready` is alive; everything else — offline, needs auth, connecting — is the
 *  quiet stopped grey. */
function StateBadge({ state }: { state: string }) {
  return (
    <span
      className={cn(
        "flex shrink-0 items-center gap-1 text-tiny font-medium tracking-[.04em] whitespace-nowrap uppercase",
        state === "ready" ? "text-status-running" : "text-status-stopped",
      )}
    >
      <span aria-hidden="true" className="size-1.25 rounded-full bg-current" />
      {state.replace(/_/g, " ")}
    </span>
  );
}

/**
 * One header row above a group or a server: the name, what the server is doing,
 * and the actions that belong to it. `title` is passed in rather than built
 * here because a server name is also a drag handle.
 */
export function SidebarSectionHeader({
  title,
  state,
  update,
  updateLabel,
  menu,
  onAdd,
  addLabel,
}: {
  title: ReactNode;
  /** Servers only — groups have no state of their own. */
  state?: string;
  update?: () => void;
  updateLabel?: string;
  menu?: ReactNode;
  onAdd?: () => void;
  addLabel?: string;
}) {
  return (
    <div className="flex items-center gap-1.5 px-2 pt-2.5 pb-0.5">
      {title}
      {state !== undefined && <StateBadge state={state} />}
      <span className="flex-1" />
      {update && (
        <Button
          size="sm"
          className="h-4.75 shrink-0 rounded-sm px-1.75 text-tiny font-semibold tracking-[.02em]"
          aria-label={updateLabel}
          title="Server update available"
          onClick={update}
        >
          Update
        </Button>
      )}
      {menu}
      {onAdd && (
        <Button
          variant="ghost"
          size="icon-xs"
          className={SECTION_ICON_CLASS}
          aria-label={addLabel}
          onClick={onAdd}
        >
          <Plus className="size-2.75" />
        </Button>
      )}
    </div>
  );
}

/** What a group or server with nothing in it says. */
export function SectionEmpty() {
  return <div className="px-2 pb-1 text-body-sm text-muted-foreground">No agents.</div>;
}
