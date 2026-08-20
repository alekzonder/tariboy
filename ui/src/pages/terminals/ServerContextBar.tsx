import { NavLink } from "react-router-dom";
import { serverPath, type ServerSection } from "@/lib/terminalsHost";
import { cn } from "@/lib/utils";

const SECTIONS: Array<{ section: ServerSection; label: string }> = [
  { section: "tasks", label: "Tasks" },
  { section: "images", label: "Images" },
  { section: "settings", label: "Settings" },
];

export function ServerContextBar({ hostId, label }: { hostId: string; label: string }) {
  return (
    <nav
      aria-label="Server workspace"
      className="flex h-12 shrink-0 items-center gap-1 border-b px-3"
    >
      <span className="mr-2 flex min-w-0 items-baseline gap-1 text-sm">
        <span className="text-muted-foreground">Server:</span>
        <span className="truncate font-semibold" title={label}>{label}</span>
      </span>
      {SECTIONS.map(({ section, label: sectionLabel }) => (
        <NavLink
          key={section}
          to={serverPath(hostId, section)}
          className={({ isActive }) => cn(
            "rounded-md border border-transparent px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground",
            isActive && "border-border bg-accent font-medium text-foreground",
          )}
        >
          {sectionLabel}
        </NavLink>
      ))}
    </nav>
  );
}
