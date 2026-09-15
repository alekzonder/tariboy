import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { hostUpdateAvailable, type DaemonMeta } from "@/lib/daemons";

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
  const state = host.state ?? "disconnected";
  const unsupported =
    (!!host.platform && host.platform !== "Linux")
    || (!!host.arch && host.arch !== "x86_64")
    || (host.prerequisites ?? []).some((item) =>
      ["Linux", "x86_64", "writable ~/.local", "flock", "python3"].includes(item),
    );
  const keyMismatch = host.message?.includes("host_key_mismatch");
  const canConnect = ["disconnected", "degraded", "failed"].includes(state);
  const updateAvailable = showUpdate && hostUpdateAvailable(host, appVersion);

  return (
    <div className="space-y-1 text-xs">
      <div className="flex items-center gap-1">
        {showState && (
          <Badge variant={state === "ready" ? "default" : "secondary"}>
            {state.replace("_", " ")}
          </Badge>
        )}
        {updateAvailable && onUpdate && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs"
            aria-label={`Update ${host.label}`}
            onClick={onUpdate}
          >
            Update
          </Button>
        )}
        {state === "needs_auth" && onUpdate && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs"
            aria-label={`Authenticate ${host.label}`}
            onClick={onUpdate}
          >
            Authenticate
          </Button>
        )}
        {canConnect && !unsupported && !keyMismatch && onConnect && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-xs"
            aria-label={`Connect ${host.label}`}
            onClick={onConnect}
          >
            Connect
          </Button>
        )}
      </div>
      {host.platform && host.arch && unsupported && (
        <div className="text-destructive">Unsupported host: {host.platform}/{host.arch}</div>
      )}
      {unsupported && <div className="font-medium text-destructive">Install blocked</div>}
      {(host.prerequisites?.length ?? 0) > 0 && (
        <div className="text-muted-foreground">
          Missing: {host.prerequisites!.join(", ")}
        </div>
      )}
      {host.message && <div className="break-words text-destructive">{host.message}</div>}
    </div>
  );
}
