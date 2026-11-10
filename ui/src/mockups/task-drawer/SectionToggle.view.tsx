import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * The chevron that opens and closes a block of the drawer header. The two
 * blocks, Details and Workflow, carry it at the same place, so they read as
 * one pair of collapsible rows.
 */
export function SectionToggle({
  label,
  open,
  onOpenChange,
}: {
  /** The name of the block: `Details` or `Workflow`. */
  label: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const action = `${open ? "Collapse" : "Expand"} ${label.toLowerCase()}`;
  return (
    <button
      type="button"
      aria-expanded={open}
      aria-label={action}
      title={action}
      onClick={() => onOpenChange(!open)}
      className="-ml-1.5 grid size-6 shrink-0 place-items-center rounded-control text-muted-foreground outline-none hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
    >
      <ChevronRight aria-hidden="true" className={cn("size-3.5", open && "rotate-90")} />
    </button>
  );
}
