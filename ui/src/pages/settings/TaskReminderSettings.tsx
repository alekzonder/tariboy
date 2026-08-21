import { useEffect, useState } from "react";
import { ApiError, defaultTaskReminderPolicy, getTaskReminderPolicyOn, setTaskReminderPolicyOn, type TaskReminderPolicy } from "@/lib/api";
import type { ApiTarget } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

function message(error: unknown): string {
  return error instanceof ApiError || error instanceof Error ? error.message : String(error);
}

function isPositiveInteger(value: string): boolean {
  const number = Number(value);
  return value.trim() !== "" && Number.isInteger(number) && number > 0;
}

export default function TaskReminderSettings({ target = null }: { target?: ApiTarget }) {
  const [enabled, setEnabled] = useState(defaultTaskReminderPolicy.enabled);
  const [threshold, setThreshold] = useState(String(defaultTaskReminderPolicy.idle_threshold_s));
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    let current = true;
    void Promise.resolve().then(() => {
      if (!current) return null;
      setLoading(true);
      setError("");
      return getTaskReminderPolicyOn(target);
    })
      .then((policy) => {
        if (!current || !policy) return;
        setEnabled(policy.enabled);
        setThreshold(String(policy.idle_threshold_s));
      })
      .catch((cause) => {
        if (current) setError(`Could not load task reminders: ${message(cause)}`);
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => { current = false; };
  }, [target]);

  const updateEnabled = (value: boolean) => {
    setEnabled(value);
    setError("");
    setNotice("");
  };
  const updateThreshold = (value: string) => {
    setThreshold(value);
    setError("");
    setNotice("");
  };

  const save = async () => {
    if (saving) return;
    if (!isPositiveInteger(threshold)) {
      setNotice("");
      setError("Enter a positive whole number of seconds.");
      return;
    }
    setSaving(true);
    setError("");
    setNotice("");
    const policy: TaskReminderPolicy = { enabled, idle_threshold_s: Number(threshold) };
    try {
      const saved = await setTaskReminderPolicyOn(target, policy);
      setEnabled(saved.enabled);
      setThreshold(String(saved.idle_threshold_s));
      setNotice("Task reminders saved");
    } catch (cause) {
      // Keep the form draft intact so the operator can correct or retry it.
      setError(message(cause));
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <p className="text-sm text-muted-foreground">Loading task reminder settings…</p>;

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="space-y-3">
        <h2 className="text-xl font-semibold">Task reminders</h2>
        <p className="text-sm text-muted-foreground">
          Send an inbox reminder when an enabled Autopilot agent has assigned open tasks and has been idle.
        </p>
      </div>
      <div className="space-y-5 rounded-lg border p-4">
        <div className="flex items-center justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="task-reminder-enabled">Enable task reminders</Label>
            <p id="task-reminder-enabled-help" className="text-xs text-muted-foreground">
              Disabled by default. Reminders use the agent’s ordinary inbox.
            </p>
          </div>
          <Switch
            id="task-reminder-enabled"
            aria-label="Enable task reminders"
            aria-describedby="task-reminder-enabled-help"
            checked={enabled}
            onCheckedChange={updateEnabled}
          />
        </div>
        <div className="max-w-xs space-y-1.5">
          <Label htmlFor="task-reminder-idle-threshold">Idle threshold (seconds)</Label>
          <Input
            id="task-reminder-idle-threshold"
            type="text"
            inputMode="numeric"
            value={threshold}
            aria-describedby="task-reminder-idle-threshold-help"
            aria-invalid={error === "Enter a positive whole number of seconds." ? true : undefined}
            onChange={(event) => updateThreshold(event.target.value)}
          />
          <p id="task-reminder-idle-threshold-help" className="text-xs text-muted-foreground">
            Use a positive whole number of seconds. The default is 300 seconds.
          </p>
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <p role="status" aria-live="polite" className="text-sm text-muted-foreground">{notice}</p>
        <Button type="button" onClick={() => void save()} disabled={saving}>
          {saving ? "Saving task reminders…" : "Save task reminders"}
        </Button>
      </div>
    </div>
  );
}
