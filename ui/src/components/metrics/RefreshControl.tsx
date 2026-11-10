import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";

const hhmm = (date: Date) =>
  date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });

/** The time of the last answer and Refresh. There is no polling: Refresh is
 *  the way to newer numbers. */
export function RefreshControl({
  updatedAt,
  updating,
  disabled,
  onRefresh,
}: {
  updatedAt: Date | null;
  updating: boolean;
  disabled: boolean;
  onRefresh: () => void;
}) {
  return (
    <div className="flex items-center gap-2 text-ui text-muted-foreground">
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
  );
}
