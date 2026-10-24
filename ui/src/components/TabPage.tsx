import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * The shared frame of page-like tabs (agent Configuration, Activity and
 * Advanced, server Activity): the same padding and block spacing everywhere.
 * Full-width pages fill the tab height so their lists can scroll inside it;
 * `narrow` centers a form-width column instead.
 */
export default function TabPage({
  narrow = false,
  className,
  children,
}: {
  narrow?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      data-testid="tab-page"
      className={cn(
        "flex min-h-0 flex-col gap-4 px-4 pt-4 pb-6",
        narrow ? "mx-auto max-w-5xl" : "h-full",
        className,
      )}
    >
      {children}
    </div>
  );
}
