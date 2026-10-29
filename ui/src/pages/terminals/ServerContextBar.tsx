import { lazy, Suspense } from "react";
import { NavLink } from "react-router-dom";
import { useActiveLinkIntoView } from "@/hooks/useActiveLinkIntoView";
import { serverPath, type ServerSection } from "@/lib/terminalsHost";
import { SECTION_ANCHORS, TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { cn } from "@/lib/utils";

// The count brings the host client and the query helpers: it loads after the shell.
const OpenMailCount = lazy(() => import("@/pages/mailbox/OpenMailCount"));

const SECTIONS: Array<{ section: ServerSection; label: string }> = [
  { section: "mail", label: "Mailbox" },
  { section: "tasks", label: "Tasks" },
  { section: "activity", label: "Activity" },
  { section: "images", label: "Images" },
  { section: "workflows", label: "Workflow images" },
  { section: "stores", label: "Stores" },
  { section: "settings", label: "Settings" },
];

/** The island's header for server-scoped views. Same strip as the agent tabs —
 *  34px, a 2px --primary underline on the active section — so the two ways of
 *  entering the island read as one control surface. */
export function ServerContextBar({ hostId, label }: { hostId: string; label: string }) {
  const strip = useActiveLinkIntoView<HTMLElement>();
  return (
    <nav
      ref={strip}
      aria-label="Server workspace"
      data-tour={TOUR_ANCHORS.serverSections}
      className="scrollbar-none flex shrink-0 items-center gap-0.5 overflow-x-auto border-b px-3"
    >
      <span className="mr-2 flex max-w-[40vw] min-w-0 shrink-0 items-baseline gap-1.5 text-body-sm">
        <span className="text-muted-foreground">Server:</span>
        <span className="truncate font-medium" title={label}>
          {label}
        </span>
      </span>
      {SECTIONS.map(({ section, label: sectionLabel }) => (
        <NavLink
          key={section}
          to={serverPath(hostId, section)}
          data-tour={SECTION_ANCHORS[section]}
          className={({ isActive }) =>
            cn(
              "flex h-8.5 shrink-0 items-center border-b-2 border-transparent px-2.5 text-body whitespace-nowrap text-muted-foreground hover:text-foreground",
              isActive && "border-primary font-medium text-foreground",
            )
          }
        >
          {sectionLabel}
          {section === "mail" && (
            <Suspense fallback={null}>
              <OpenMailCount hostId={hostId} />
            </Suspense>
          )}
        </NavLink>
      ))}
    </nav>
  );
}
