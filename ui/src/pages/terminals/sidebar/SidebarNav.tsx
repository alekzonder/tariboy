import { Link, useLocation } from "react-router-dom";
import {
  ChevronDown,
  ChevronRight,
  Inbox,
  Library,
  ListChecks,
  Settings,
  Users,
  type LucideIcon,
} from "lucide-react";
import { activeNav, navPath, type NavItem } from "@/lib/navRoutes";
import { TOUR_ANCHORS, type TourAnchor } from "@/lib/onboarding/tour";
import { cn } from "@/lib/utils";

const ROW_CLASS =
  "flex h-7.5 w-full items-center gap-2 rounded-control px-2 text-body text-muted-foreground hover:bg-sidebar-accent hover:text-foreground";
const ICON_CLASS = "size-3.75 shrink-0";

/** A count beside a nav item: what waits for the user there. */
function Badge({ count, label }: { count: number; label: string }) {
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className="ml-auto rounded-full bg-primary px-1.5 text-caption leading-4 font-medium text-primary-foreground"
    >
      {count}
    </span>
  );
}

/** One sidebar item: a link with an icon, active when its page is open. */
export function SidebarNavItem({
  to,
  label,
  icon: Icon,
  active,
  badge,
  tour,
  onNavigate,
}: {
  to: string;
  label: string;
  icon: LucideIcon;
  active: boolean;
  /** Shown only when above zero. */
  badge?: { count: number | null; label: string };
  tour?: TourAnchor;
  onNavigate?: () => void;
}) {
  return (
    <Link
      to={to}
      aria-current={active ? "page" : undefined}
      data-tour={tour}
      onClick={onNavigate}
      className={cn(ROW_CLASS, active && "bg-accent font-medium text-foreground")}
    >
      <Icon className={ICON_CLASS} aria-hidden="true" />
      <span className="min-w-0 truncate">{label}</span>
      {badge && badge.count !== null && badge.count > 0 && (
        <Badge count={badge.count} label={badge.label} />
      )}
    </Link>
  );
}

/** Tasks, Inbox, Library and Settings below the agent list, with the counts
 *  of what waits for the user. */
export function SectionsNav({
  pageHostId,
  counts,
  onNavigate,
}: {
  /** The host Library and Settings open on. */
  pageHostId: string;
  counts: { tasks: number | null; inbox: number | null };
  onNavigate?: () => void;
}) {
  const active = activeNav(useLocation().pathname);
  const item = (id: NavItem, label: string, icon: LucideIcon) => ({
    to: navPath(id, pageHostId),
    label,
    icon,
    onNavigate,
  });
  return (
    <nav aria-label="Sections" className="flex shrink-0 flex-col gap-0.5 px-1.5 pt-1.5">
      <SidebarNavItem
        {...item("tasks", "Tasks", ListChecks)}
        active={active === "tasks"}
        tour={TOUR_ANCHORS.sectionTasks}
        badge={{ count: counts.tasks, label: `${counts.tasks} tasks need you` }}
      />
      <SidebarNavItem
        {...item("inbox", "Inbox", Inbox)}
        active={active === "inbox"}
        badge={{ count: counts.inbox, label: `${counts.inbox} open letters` }}
      />
      <SidebarNavItem
        {...item("library-images", "Library", Library)}
        active={active === "library"}
        tour={TOUR_ANCHORS.sectionImages}
      />
      <SidebarNavItem {...item("settings", "Settings", Settings)} active={active === "settings"} />
    </nav>
  );
}

/** The Agents heading: not a page, it opens and closes the list below it. */
export function AgentsHeading({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  const Chevron = open ? ChevronDown : ChevronRight;
  return (
    <button type="button" aria-expanded={open} onClick={onToggle} className={ROW_CLASS}>
      <Users className={ICON_CLASS} aria-hidden="true" />
      <span className="min-w-0 truncate">Agents</span>
      <Chevron className="ml-auto size-3.25 shrink-0" aria-hidden="true" />
    </button>
  );
}
