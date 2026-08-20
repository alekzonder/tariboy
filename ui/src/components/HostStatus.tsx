import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { DaemonMeta } from "@/lib/daemons";

export function HostStatus({
  host,
  appVersion,
  onConnect,
  onUpdate,
}: {
  host: DaemonMeta;
  appVersion: string;
  onConnect?: () => void;
  onUpdate?: () => void;
}) {
  if (host.kind !== "ssh") return null;
  const state = host.state ?? "disconnected";
  const unsupported =
    (!!host.platform && host.platform !== "Linux")
    || (!!host.arch && host.arch !== "x86_64")
    || (host.prerequisites ?? []).some((item) =>
      ["Linux", "x86_64", "writable ~/.local", "flock"].includes(item),
    );
  const keyMismatch = host.message?.includes("host_key_mismatch");
  const canConnect = ["disconnected", "degraded", "failed"].includes(state);
  const remoteVersion = host.lastDaemonVersion ?? "";
  const updateAvailable =
    state === "ready"
    && appVersion !== ""
    && remoteVersion !== ""
    && remoteVersion !== appVersion;

  return (
    <div className="space-y-1 text-xs">
      <div className="flex items-center gap-1">
        <Badge variant={state === "ready" ? "default" : "secondary"}>
          {state.replace("_", " ")}
        </Badge>
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
