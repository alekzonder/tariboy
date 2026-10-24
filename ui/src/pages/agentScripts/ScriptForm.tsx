import { type FormEvent, type ReactNode, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { ScriptMode } from "@/lib/api";
import { emptyScriptForm, scriptRequest } from "./scriptRequest";
import type { LaunchRequest } from "./useAgentScripts";

function Field({
  label,
  className = "",
  children,
}: {
  label: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <label className={`grid gap-1 text-sm ${className}`}>
      <Label>{label}</Label>
      {children}
    </label>
  );
}

function ModeButton({
  mode,
  current,
  onSelect,
  children,
}: {
  mode: ScriptMode;
  current: ScriptMode;
  onSelect: (mode: ScriptMode) => void;
  children: ReactNode;
}) {
  return (
    <Button
      type="button"
      variant={current === mode ? "default" : "outline"}
      aria-pressed={current === mode}
      onClick={() => onSelect(mode)}
    >
      {children}
    </Button>
  );
}

// ScriptForm is the launch form for a one-shot or scheduled local script.
export function ScriptForm({
  onLaunch,
}: {
  onLaunch: (request: LaunchRequest, clearForm: () => void) => Promise<void>;
}) {
  const [form, setForm] = useState(emptyScriptForm);
  const [busy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const request = scriptRequest(form);
    if ("error" in request) {
      toast.error(request.error);
      return;
    }
    setBusy(true);
    try {
      await onLaunch(request, () => setForm(emptyScriptForm));
    } finally {
      setBusy(false);
    }
  };
  const selectMode = (mode: ScriptMode) => setForm({ ...form, mode });

  return (
    <section className="rounded-lg border p-4">
      <h2 className="text-lg font-semibold">Local scripts</h2>
      <div className="mt-3 flex gap-2" aria-label="Script kind">
        <ModeButton mode="once" current={form.mode} onSelect={selectMode}>
          Run once
        </ModeButton>
        <ModeButton mode="every" current={form.mode} onSelect={selectMode}>
          Schedule
        </ModeButton>
      </div>
      <form className="mt-4 grid gap-3 sm:grid-cols-2" onSubmit={submit}>
        <Field label="Name">
          <Input
            required
            value={form.script_name}
            onChange={(event) => setForm({ ...form, script_name: event.target.value })}
          />
        </Field>
        <Field label="Description">
          <Input
            required
            value={form.description}
            onChange={(event) => setForm({ ...form, description: event.target.value })}
          />
        </Field>
        <Field label="Command" className="sm:col-span-2">
          <Textarea
            required
            value={form.command}
            onChange={(event) => setForm({ ...form, command: event.target.value })}
          />
        </Field>
        {form.mode === "every" && (
          <>
            <Field label="Every (seconds)">
              <Input
                required
                min="1"
                type="number"
                value={form.interval_seconds}
                onChange={(event) => setForm({ ...form, interval_seconds: event.target.value })}
              />
            </Field>
            <p className="self-end text-xs text-muted-foreground">
              A scheduled run that exits 111 publishes nothing and keeps the schedule running.
            </p>
          </>
        )}
        <div className="sm:col-span-2">
          <Button disabled={busy} type="submit">
            {form.mode === "once" ? "Run once" : "Schedule"}
          </Button>
        </div>
      </form>
    </section>
  );
}
