import { useCallback, useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import { guard } from "@/lib/toast-guard";
import type { PluginList, PluginLogs, PluginSummary } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

function PluginRow({
  plugin: pl,
  onLogs,
  onChanged,
}: {
  plugin: PluginSummary;
  onLogs: () => void;
  onChanged: () => void;
}) {
  const api = useApi();
  return (
    <tr className="border-t">
      <td className="px-3 py-1.5 font-mono">{pl.name}</td>
      <td className="px-3 py-1.5">{pl.version}</td>
      <td className="px-3 py-1.5 text-xs text-muted-foreground">{(pl.types || []).join(", ")}</td>
      <td className="px-3 py-1.5">
        {pl.state === "running" ? (
          <Badge>running</Badge>
        ) : (
          <Badge variant="secondary">{pl.state}</Badge>
        )}
      </td>
      <td className="px-3 py-1.5 text-right">
        <Button size="sm" variant="ghost" onClick={onLogs}>
          Logs
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() =>
            guard(
              "restart",
              () => api.post(`/api/plugins/${encodeURIComponent(pl.name)}/restart`),
              { verb: "ok", after: onChanged },
            )
          }
        >
          Restart
        </Button>
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button size="sm" variant="destructive">
              Remove
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Remove plugin {pl.name}?</AlertDialogTitle>
              <AlertDialogDescription>Stops and removes the plugin (force).</AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                onClick={() =>
                  guard(
                    "remove",
                    () => api.delete(`/api/plugins/${encodeURIComponent(pl.name)}?force=true`),
                    { verb: "ok", after: onChanged },
                  )
                }
              >
                Remove
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </td>
    </tr>
  );
}

export default function PluginsPage() {
  const api = useApi();
  const [plugins, setPlugins] = useState<PluginSummary[]>([]);
  const [logs, setLogs] = useState<{ name: string; lines: string[] } | null>(null);
  const [path, setPath] = useState("");

  const load = useCallback(
    () =>
      api
        .get<PluginList>("/api/plugins")
        .then((r) => setPlugins(r.plugins))
        .catch(() => setPlugins([])),
    [api],
  );
  const showLogs = (name: string) =>
    api
      .get<PluginLogs>(`/api/plugins/${encodeURIComponent(name)}/logs`)
      .then((r) => setLogs({ name, lines: r.lines ?? [] }))
      .catch(() => setLogs({ name, lines: [] }));
  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="space-y-4 p-6">
      <h1 className="text-lg font-semibold">Plugins</h1>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Install plugin</CardTitle>
        </CardHeader>
        <CardContent className="flex items-end gap-2">
          <Input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="daemon-accessible plugin dir"
            className="h-8 w-96 min-w-0"
          />
          <Button
            size="sm"
            onClick={() =>
              guard("install", () => api.post("/api/plugins", { path }), {
                verb: "ok",
                after: () => {
                  setPath("");
                  void load();
                },
              })
            }
          >
            Install
          </Button>
        </CardContent>
      </Card>
      <div className="overflow-x-auto rounded border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2">Name</th>
              <th className="px-3 py-2">Version</th>
              <th className="px-3 py-2">Types</th>
              <th className="px-3 py-2">State</th>
              <th className="px-3 py-2"></th>
            </tr>
          </thead>
          <tbody>
            {plugins.map((pl) => (
              <PluginRow
                key={pl.name}
                plugin={pl}
                onLogs={() => void showLogs(pl.name)}
                onChanged={() => void load()}
              />
            ))}
            {plugins.length === 0 && (
              <tr>
                <td colSpan={5} className="px-3 py-4 text-center text-muted-foreground">
                  No plugins.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {logs && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base font-mono">{logs.name} logs</CardTitle>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-64 rounded border bg-muted/30 p-2">
              <pre className="whitespace-pre-wrap text-xs">
                {logs.lines.join("\n") || "(no logs)"}
              </pre>
            </ScrollArea>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
