import type { ReactNode } from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";

const hhmm = (date: Date) =>
  date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });

/** A metrics screen's title, with Refresh and the time of the last answer on
 *  the right. There is no polling: Refresh is the way to newer numbers. */
export function PageHeading({
  title,
  subtitle,
  updatedAt,
  updating,
  disabled,
  onRefresh,
}: {
  title: ReactNode;
  subtitle: ReactNode;
  updatedAt: Date | null;
  updating: boolean;
  disabled: boolean;
  onRefresh: () => void;
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="text-sm break-words text-muted-foreground">{subtitle}</p>
      </div>
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        {updatedAt && <span>Updated {hhmm(updatedAt)}</span>}
        <Button
          type="button"
          size="sm"
          variant="outline"
          aria-busy={updating}
          disabled={disabled}
          onClick={onRefresh}
        >
          <RefreshCw aria-hidden="true" />
          {updating ? "Updating…" : "Refresh"}
        </Button>
      </div>
    </div>
  );
}
