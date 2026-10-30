import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { TOUR_ANCHORS, type TourAnchor } from "@/lib/onboarding/tour";
import { serverPath, type ServerSection } from "@/lib/terminalsHost";
import { cn } from "@/lib/utils";

const TABS: Array<{ section: ServerSection; label: string; tour?: TourAnchor }> = [
  { section: "images", label: "Images" },
  { section: "workflows", label: "Workflows", tour: TOUR_ANCHORS.sectionWorkflows },
  { section: "stores", label: "Sources", tour: TOUR_ANCHORS.sectionStores },
];

/** The Library's tabs over Images, Workflows and Sources of one host, and the
 *  host selector when the workspace has several. */
export function LibraryHeader({
  hostId,
  hostControl,
}: {
  hostId: string;
  hostControl?: ReactNode;
}) {
  return (
    <nav
      aria-label="Library"
      data-toolbar
      className="flex shrink-0 flex-nowrap items-center gap-0.5 border-b px-3"
    >
      {TABS.map(({ section, label, tour }) => (
        <NavLink
          key={section}
          to={serverPath(hostId, section)}
          data-tour={tour}
          className={({ isActive }) =>
            cn(
              "flex h-8.5 shrink-0 items-center border-b-2 border-transparent px-2.5 text-body whitespace-nowrap text-muted-foreground hover:text-foreground",
              isActive && "border-primary font-medium text-foreground",
            )
          }
        >
          {label}
        </NavLink>
      ))}
      {hostControl && <div className="ml-auto py-1">{hostControl}</div>}
    </nav>
  );
}
