import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { isDesktop, supportBundleExport } from "@/lib/desktop";
import { getActiveId, listDaemons } from "@/lib/daemons";

interface SupportHost {
  id: string;
  label: string;
  kind: "local" | "ssh" | "https";
}

const LOCAL_HOST: SupportHost = { id: "local", label: "Local", kind: "local" };

export function SupportBundle() {
  const available = isDesktop();
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(available);
  const [hosts, setHosts] = useState<SupportHost[]>([LOCAL_HOST]);
  const [hostId, setHostId] = useState("local");
  const [includeAgentData, setIncludeAgentData] = useState(false);
  const [path, setPath] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    if (!available) {
      return () => {
        cancelled = true;
      };
    }
    void Promise.all([listDaemons(), getActiveId()])
      .then(([remoteHosts, activeId]) => {
        if (cancelled) return;
        const options: SupportHost[] = [
          LOCAL_HOST,
          ...remoteHosts
            .filter((host) => host.kind === "ssh" || host.kind === "https")
            .map((host) => ({
              id: host.id,
              label: host.label,
              kind: host.kind as "ssh" | "https",
            })),
        ];
        setHosts(options);
        setHostId(options.some((host) => host.id === activeId) ? activeId : "local");
      })
      .catch((cause) => {
        if (!cancelled) {
          setError(`Could not load hosts: ${cause instanceof Error ? cause.message : String(cause)}`);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [available]);

  const run = async () => {
    setBusy(true);
    setPath("");
    setError("");
    try {
      const result = await supportBundleExport(hostId, includeAgentData);
      if (result) setPath(result.path);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Support bundle</CardTitle>
        <CardDescription>
          Export a bounded, privacy-safe ZIP for troubleshooting.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <div className="space-y-1">
          <label className="font-medium" htmlFor="support-host">Host</label>
          <select
            id="support-host"
            className="block h-9 w-full rounded-md border bg-background px-3"
            value={hostId}
            disabled={!available || loading || busy}
            onChange={(event) => setHostId(event.target.value)}
          >
            {hosts.map((host) => (
              <option key={host.id} value={host.id}>
                {host.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <div className="font-medium">Included by default</div>
          <p className="text-muted-foreground">
            App details plus the selected host ID, label, health, prerequisites, daemon diagnostics,
            and redacted lifecycle log lines. Other hosts are not collected.
          </p>
        </div>
        <div className="space-y-1 rounded-md border p-3">
          <label className="flex items-center gap-2 font-medium">
            <input
              type="checkbox"
              checked={includeAgentData}
              disabled={!available || loading || busy}
              onChange={(event) => setIncludeAgentData(event.target.checked)}
            />
            Include agent data (sensitive)
          </label>
          <p className="text-muted-foreground">
            Adds complete redacted result, shim, stdout, and stderr files from the newest 10 iterations
            for each agent on this host. Agent and model output can contain private text; inspect the ZIP
            before sharing it.
          </p>
        </div>
        <div>
          <div className="font-medium">Always excluded</div>
          <p className="text-muted-foreground">
            The collector never includes PROMPT.md, transcripts, secrets, environment values, workdirs,
            or user files. SSH aliases, configuration, audit, context, image, and provisioning data are
            also excluded.
          </p>
        </div>
        <Button disabled={!available || loading || busy || !hostId} onClick={() => void run()}>
          {busy ? "Exporting…" : "Export support bundle"}
        </Button>
        {!available && <p className="text-muted-foreground">Support export is available in the desktop app.</p>}
        {path && <p role="status" className="break-all text-muted-foreground">Saved to {path}</p>}
        {error && <p role="alert" className="text-destructive">Export failed: {error}</p>}
      </CardContent>
    </Card>
  );
}
