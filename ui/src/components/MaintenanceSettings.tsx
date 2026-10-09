import { useEffect, useState } from "react";
import {
  getIterationRetentionDefaultOn, getMaintenanceOn, runMaintenanceOn, setIterationRetentionDefaultOn, setMaintenanceOn,
  type ApiTarget, type IterationRetention, type MaintenanceRun, type MaintenanceSettings,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

const reason = (e: unknown) => (e instanceof Error ? e.message : String(e));
const mib = (n: number) => `${(n / 1048576).toFixed(1)} MiB`;

function describeRun(run: MaintenanceRun) {
  const deleted = Object.entries(run.deleted ?? {})
    .filter(([, n]) => n > 0)
    .map(([k, n]) => `${k} ${n}`)
    .join(", ");
  return [
    `${run.finished_at} (${run.trigger})`,
    `deleted: ${deleted || "nothing"}`,
    `${mib(run.size_before)} → ${mib(run.size_after)}${run.compacted ? " (compacted)" : ""}`,
  ].join(" · ");
}

// Mount with a per-host key so a draft for one host is never saved to another.
export function MaintenanceSettingsCard({ target }: { target: ApiTarget }) {
  const [form, setForm] = useState<MaintenanceSettings | null>(null);
  const [saved, setSaved] = useState<MaintenanceSettings | null>(null);
  const [ret, setRet] = useState<IterationRetention | null>(null);
  const [savedRet, setSavedRet] = useState<IterationRetention | null>(null);
  const [lastRun, setLastRun] = useState<MaintenanceRun | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let current = true;
    getMaintenanceOn(target)
      .then((r) => {
        if (!current) return;
        setForm(r.settings);
        setSaved(r.settings);
        setLastRun(r.last_run);
        setError(r.last_run?.error ?? "");
      })
      .catch((e) => { if (current) setError(reason(e)); });
    getIterationRetentionDefaultOn(target)
      .then((r) => {
        if (!current) return;
        const p = { keep_iterations: r.keep_iterations, keep_days: r.keep_days, max_bytes: r.max_bytes };
        setRet(p);
        setSavedRet(p);
      })
      .catch((e) => { if (current) setError(reason(e)); });
    return () => { current = false; };
  }, [target]);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try { await fn(); } catch (e) { setError(reason(e)); }
    setBusy(false);
  };
  const save = () => act(async () => {
    if (!form) return;
    const next = await setMaintenanceOn(target, form);
    setForm(next);
    setSaved(next);
    if (!ret) return;
    const nextRet = await setIterationRetentionDefaultOn(target, ret);
    setRet(nextRet);
    setSavedRet(nextRet);
  });
  const run = () => act(async () => {
    const r = await runMaintenanceOn(target);
    setLastRun(r);
    if (r.error) setError(r.error);
  });
  const patch = (p: Partial<MaintenanceSettings>) => setForm((f) => (f ? { ...f, ...p } : f));
  const patchRet = (p: Partial<IterationRetention>) => setRet((r) => (r ? { ...r, ...p } : r));
  const num = (v: string) => Number.parseInt(v, 10) || 0;
  const dirty = JSON.stringify(form) !== JSON.stringify(saved) || JSON.stringify(ret) !== JSON.stringify(savedRet);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Backup & data retention</CardTitle>
        <CardDescription>
          Every night the whole database is backed up; after a successful backup, agent iterations
          beyond the iteration limits are archived with their database rows and deleted, finished tasks,
          AI proxy usage, delivered messages, and events older than the retention period are deleted,
          and the database is compacted.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {form && (
          <>
            <div className="flex items-center gap-2">
              <Switch id="maint-enabled" checked={form.enabled} onCheckedChange={(v) => patch({ enabled: v })} />
              <Label htmlFor="maint-enabled">Nightly backup and cleanup</Label>
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="maint-time">Nightly run time</Label>
                <Input id="maint-time" type="time" className="h-9" value={form.time}
                  onChange={(e) => patch({ time: e.target.value })} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="maint-days">Keep data for (days)</Label>
                <Input id="maint-days" inputMode="numeric" className="h-9" value={String(form.retention_days)}
                  aria-describedby="maint-days-help" onChange={(e) => patch({ retention_days: num(e.target.value) })} />
                <p id="maint-days-help" className="text-xs text-muted-foreground">0 keeps everything; otherwise at least 31.</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="maint-backups">Backups to keep</Label>
                <Input id="maint-backups" inputMode="numeric" className="h-9" value={String(form.keep_backups)}
                  onChange={(e) => patch({ keep_backups: num(e.target.value) })} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="maint-threshold">Compact when free space reaches (%)</Label>
                <Input id="maint-threshold" inputMode="numeric" className="h-9" value={String(form.compact_threshold_pct)}
                  onChange={(e) => patch({ compact_threshold_pct: num(e.target.value) })} />
              </div>
            </div>
            {ret && (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                <div className="space-y-1.5">
                  <Label htmlFor="ret-keep-it">Keep iterations per agent</Label>
                  <Input id="ret-keep-it" inputMode="numeric" className="h-9" value={String(ret.keep_iterations)}
                    aria-describedby="ret-help" onChange={(e) => patchRet({ keep_iterations: num(e.target.value) })} />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="ret-keep-days">Keep iterations for (days)</Label>
                  <Input id="ret-keep-days" inputMode="numeric" className="h-9" value={String(ret.keep_days)}
                    aria-describedby="ret-help" onChange={(e) => patchRet({ keep_days: num(e.target.value) })} />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="ret-max-bytes">Max iteration bytes per agent</Label>
                  <Input id="ret-max-bytes" inputMode="numeric" className="h-9" value={String(ret.max_bytes)}
                    aria-describedby="ret-help" onChange={(e) => patchRet({ max_bytes: num(e.target.value) })} />
                </div>
                <p id="ret-help" className="text-xs text-muted-foreground sm:col-span-3">
                  0 is unlimited. A non-zero value in an agent&apos;s retention settings overrides these.
                </p>
              </div>
            )}
            <div className="flex items-center gap-2">
              <Switch id="maint-compact" checked={form.compact} onCheckedChange={(v) => patch({ compact: v })} />
              <Label htmlFor="maint-compact">Compact after cleanup</Label>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" disabled={busy} onClick={() => void save()}>Save</Button>
              <Button size="sm" variant="secondary" disabled={busy || dirty} onClick={() => void run()}>Run now</Button>
            </div>
            {dirty && <p className="text-xs text-muted-foreground">Save before running: Run now uses the saved settings.</p>}
          </>
        )}
        {lastRun && <p className="text-xs text-muted-foreground">Last run: {describeRun(lastRun)}</p>}
        {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
      </CardContent>
    </Card>
  );
}
