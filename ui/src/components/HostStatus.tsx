import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { DaemonMeta } from "@/lib/daemons";
import { HostStatusNotes } from "./HostStatusNotes";
import { hostStatusView } from "./hostStatusModel";

export function HostStatus({
  host,
  appVersion,
  onConnect,
  onUpdate,
  showUpdate = true,
  showState = true,
}: {
  host: DaemonMeta;
  appVersion: string;
  onConnect?: () => void;
  onUpdate?: () => void;
  /** false where the surrounding header already offers the update — the
   *  sidebar's server row does. Authentication and connection stay here. */
  showUpdate?: boolean;
  /** false where the surrounding header already names the state, so it is not
   *  said twice. */
  showState?: boolean;
}) {
  if (host.kind !== "ssh") return null;
  const { state, unsupported, badgeVariant, actions } = hostStatusView(
    host,
    appVersion,
    showUpdate,
  );
  const handlers = { onUpdate, onConnect };

  return (
    <div className="space-y-1 text-xs">
      <div className="flex items-center gap-1">
        {showState && <Badge variant={badgeVariant}>{state.replace("_", " ")}</Badge>}
        {actions.map(({ label, handler }) => {
          const onClick = handlers[handler];
          return (
            onClick && (
              <Button
                key={label}
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-xs"
                aria-label={`${label} ${host.label}`}
                onClick={onClick}
              >
                {label}
              </Button>
            )
          );
        })}
      </div>
      <HostStatusNotes host={host} unsupported={unsupported} />
    </div>
  );
}
