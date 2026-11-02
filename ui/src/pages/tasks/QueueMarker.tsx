import { Layers } from "lucide-react";
import { cn } from "@/lib/utils";

/** Says that a task's queue differs from its parent's; the label names both.
 *  It is an image, not a control, so it takes no focus. */
export default function QueueMarker({ label, className }: { label: string; className?: string }) {
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className={cn("inline-flex shrink-0 items-center text-muted-foreground", className)}
    >
      <Layers aria-hidden="true" className="size-3" />
    </span>
  );
}
