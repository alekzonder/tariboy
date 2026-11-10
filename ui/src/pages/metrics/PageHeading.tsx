import type { ComponentProps, ReactNode } from "react";
import { RefreshControl } from "@/components/metrics/RefreshControl";

/** A metrics screen's title, with Refresh and the time of the last answer on
 *  the right. */
export function PageHeading({
  title,
  subtitle,
  ...refresh
}: {
  title: ReactNode;
  subtitle: ReactNode;
} & ComponentProps<typeof RefreshControl>) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0">
        <h1 className="text-heading font-semibold">{title}</h1>
        <p className="text-body break-words text-muted-foreground">{subtitle}</p>
      </div>
      <RefreshControl {...refresh} />
    </div>
  );
}
