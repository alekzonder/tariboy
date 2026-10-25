import { useEffect, useState } from "react";
import type { RetentionPolicy, MaintenanceRun, MaintenanceSettings } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { errorText } from "@/lib/errors";

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

const num = (v: string) => Number.parseInt(v, 10) || 0;

/** A labelled whole-number input; its help text, if any, describes it. */
function NumberField({
  id,
  label,
  value,
  help,
  describedBy,
  onChange,
}: {
  id: string;
  label: string;
  value: number;
  help?: string;
  describedBy?: string;
  onChange: (value: number) => void;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        inputMode="numeric"
        className="h-9"
        value={String(value)}
        aria-describedby={help ? `${id}-help` : describedBy}
        onChange={(e) => onChange(num(e.target.value))}
      />
      {help && (
        <p id={`${id}-help`} className="text-xs text-muted-foreground">
          {help}
        </p>
      )}
    </div>
  );
}

/** The nightly schedule and the retention periods of the maintenance settings. */
function MaintenanceFields({
  form,
  patch,
}: {
  form: MaintenanceSettings;
  patch: (p: Partial<MaintenanceSettings>) => void;
}) {
  return (
    <>
      <div className="flex items-center gap-2">
        <Switch
          id="maint-enabled"
          checked={form.enabled}
          onCheckedChange={(v) => patch({ enabled: v })}
        />
        <Label htmlFor="maint-enabled">Nightly backup and cleanup</Label>
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="maint-time">Nightly run time</Label>
          <Input
            id="maint-time"
            type="time"
            className="h-9"
            value={form.time}
            onChange={(e) => patch({ time: e.target.value })}
          />
        </div>
        <NumberField
          id="maint-days"
          label="Keep data for (days)"
          value={form.retention_days}
          help="0 keeps everything; otherwise at least 31."
          onChange={(n) => patch({ retention_days: n })}
        />
        <NumberField
          id="maint-chat-days"
          label="Keep chat history for (days)"
          value={form.chat_retention_days}
          help="0 keeps chat history for the data period."
          onChange={(n) => patch({ chat_retention_days: n })}
        />
        <NumberField
          id="maint-backups"
          label="Backups to keep"
          value={form.keep_backups}
          onChange={(n) => patch({ keep_backups: n })}
        />
        <NumberField
          id="maint-threshold"
          label="Compact when free space reaches (%)"
          value={form.compact_threshold_pct}
          onChange={(n) => patch({ compact_threshold_pct: n })}
        />
      </div>
    </>
  );
}

/** The daemon-wide iteration retention that agents without their own limits use. */
function RetentionDefaults({
  ret,
  patchRet,
}: {
  ret: RetentionPolicy;
  patchRet: (p: Partial<RetentionPolicy>) => void;
}) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
      <NumberField
        id="ret-keep-it"
        label="Keep iterations per agent"
        value={ret.keep_iterations}
        describedBy="ret-help"
        onChange={(n) => patchRet({ keep_iterations: n })}
      />
      <NumberField
        id="ret-keep-days"
        label="Keep iterations for (days)"
        value={ret.keep_days}
        describedBy="ret-help"
        onChange={(n) => patchRet({ keep_days: n })}
      />
      <NumberField
        id="ret-max-bytes"
        label="Max iteration bytes per agent"
        value={ret.max_bytes}
        describedBy="ret-help"
        onChange={(n) => patchRet({ max_bytes: n })}
      />
      <p id="ret-help" className="text-xs text-muted-foreground sm:col-span-3">
        0 is unlimited. A non-zero value in an agent&apos;s retention settings overrides these.
      </p>
    </div>
  );
}

// Mount with a per-host key (and inside that host's ApiProvider) so a draft for
// one host is never saved to another.
export function MaintenanceSettingsCard() {
  const api = useApi();
  const [form, setForm] = useState<MaintenanceSettings | null>(null);
  const [saved, setSaved] = useState<MaintenanceSettings | null>(null);
  const [ret, setRet] = useState<RetentionPolicy | null>(null);
  const [savedRet, setSavedRet] = useState<RetentionPolicy | null>(null);
  const [lastRun, setLastRun] = useState<MaintenanceRun | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let current = true;
    api.daemon
      .getMaintenance()
      .then((r) => {
        if (!current) return;
        setForm(r.settings);
        setSaved(r.settings);
        setLastRun(r.last_run);
        setError(r.last_run?.error ?? "");
      })
      .catch((e) => {
        if (current) setError(errorText(e));
      });
    api.daemon
      .getIterationRetentionDefault()
      .then((r) => {
        if (!current) return;
        const p = {
          keep_iterations: r.keep_iterations,
          keep_days: r.keep_days,
          max_bytes: r.max_bytes,
        };
        setRet(p);
        setSavedRet(p);
      })
      .catch((e) => {
        if (current) setError(errorText(e));
      });
    return () => {
      current = false;
    };
  }, [api]);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(errorText(e));
    }
    setBusy(false);
  };
  const save = () =>
    act(async () => {
      if (!form) return;
      const next = await api.daemon.setMaintenance(form);
      setForm(next);
      setSaved(next);
      if (!ret) return;
      const nextRet = await api.daemon.setIterationRetentionDefault(ret);
      setRet(nextRet);
      setSavedRet(nextRet);
    });
  const run = () =>
    act(async () => {
      const r = await api.daemon.runMaintenance();
      setLastRun(r);
      if (r.error) setError(r.error);
    });
  const patch = (p: Partial<MaintenanceSettings>) => setForm((f) => (f ? { ...f, ...p } : f));
  const patchRet = (p: Partial<RetentionPolicy>) => setRet((r) => (r ? { ...r, ...p } : r));
  const dirty =
    JSON.stringify(form) !== JSON.stringify(saved) ||
    JSON.stringify(ret) !== JSON.stringify(savedRet);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Backup & data retention</CardTitle>
        <CardDescription>
          Every night the whole database is backed up; after a successful backup, agent iterations
          beyond the iteration limits are archived with their database rows and deleted, finished
          tasks, AI proxy usage, and events older than the retention period are deleted, delivered
          chat messages of all agents older than the chat history period are deleted, and the
          database is compacted.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {form && (
          <>
            <MaintenanceFields form={form} patch={patch} />
            {ret && <RetentionDefaults ret={ret} patchRet={patchRet} />}
            <div className="flex items-center gap-2">
              <Switch
                id="maint-compact"
                checked={form.compact}
                onCheckedChange={(v) => patch({ compact: v })}
              />
              <Label htmlFor="maint-compact">Compact after cleanup</Label>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" disabled={busy} onClick={() => void save()}>
                Save
              </Button>
              <Button
                size="sm"
                variant="secondary"
                disabled={busy || dirty}
                onClick={() => void run()}
              >
                Run now
              </Button>
            </div>
            {dirty && (
              <p className="text-xs text-muted-foreground">
                Save before running: Run now uses the saved settings.
              </p>
            )}
          </>
        )}
        {lastRun && (
          <p className="text-xs text-muted-foreground">Last run: {describeRun(lastRun)}</p>
        )}
        {error && (
          <p role="alert" className="text-xs text-destructive">
            {error}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
