import type { DaemonMeta } from "@/lib/daemons";

export function HostStatusNotes({ host, unsupported }: { host: DaemonMeta; unsupported: boolean }) {
  return (
    <>
      {host.platform && host.arch && unsupported && (
        <div className="text-destructive">
          Unsupported host: {host.platform}/{host.arch}
        </div>
      )}
      {unsupported && <div className="font-medium text-destructive">Install blocked</div>}
      {(host.prerequisites?.length ?? 0) > 0 && (
        <div className="text-muted-foreground">Missing: {host.prerequisites!.join(", ")}</div>
      )}
      {host.message && <div className="break-words text-destructive">{host.message}</div>}
    </>
  );
}
