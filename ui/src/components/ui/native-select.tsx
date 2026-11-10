import { ChevronDown } from "lucide-react";
import type { ComponentProps, ReactNode } from "react";

import { cn } from "@/lib/utils";
import { FIELD } from "./recipes";

/**
 * A native select wearing the field fill, with the chevron drawn over it. Use
 * it where the options are plain text and the platform picker is the better
 * control (a phone, a long list); `Select` is the styled menu.
 */
export function NativeSelect({
  className,
  children,
  ...props
}: ComponentProps<"select"> & { children: ReactNode }) {
  return (
    <span data-slot="native-select" className="relative inline-flex min-w-0">
      <select className={cn(FIELD, "appearance-none pr-7", className)} {...props}>
        {children}
      </select>
      <ChevronDown
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 right-2.5 size-2.5 -translate-y-1/2 opacity-45"
      />
    </span>
  );
}
