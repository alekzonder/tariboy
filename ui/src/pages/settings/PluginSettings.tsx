import { useEffect, useState } from "react";
import {
  ApiError,
  getPluginContributionsOn,
  getPluginStatusOn,
  runPluginActionOn,
  type ApiTarget,
  type PluginContribution,
  type PluginSettingField,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

function message(error: unknown): string {
  return error instanceof ApiError || error instanceof Error ? error.message : String(error);
}

function statusValue(value: unknown): string {
  if (typeof value === "boolean") return value ? "Yes" : "No";
  if (value === null || value === undefined || value === "") return "—";
  return String(value);
}

function integerList(field: PluginSettingField, value: string): number[] {
  if (value.trim() === "") return [];
  return value.split(",").map((part) => {
    const trimmed = part.trim();
    if (!/^-?\d+$/.test(trimmed) || !Number.isSafeInteger(Number(trimmed))) {
      throw new Error(`${field.label} must be a comma-separated integer list`);
    }
    return Number(trimmed);
  });
}

export default function PluginSettings({ name, target = null }: { name: string; target?: ApiTarget }) {
  const [plugin, setPlugin] = useState<PluginContribution>();
  const [status, setStatus] = useState<Record<string, unknown>>({});
  const [values, setValues] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const loadStatus = async () => setStatus(await getPluginStatusOn(target, name));

  useEffect(() => {
    let current = true;
    void Promise.resolve().then(() => {
      if (!current) return null;
      setLoading(true);
      setError("");
      return Promise.all([getPluginContributionsOn(target), getPluginStatusOn(target, name)]);
    })
      .then((result) => {
        if (!current || !result) return;
        const [contributions, nextStatus] = result;
        setPlugin(contributions.plugins.find((item) => item.name === name));
        setStatus(nextStatus);
      })
      .catch((cause) => { if (current) setError(message(cause)); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [name, target]);

  const act = async (action: string, fieldNames: string[], fields: PluginSettingField[]) => {
    if (saving) return;
    setError("");
    setNotice("");
    const data: Record<string, unknown> = {};
    try {
      for (const fieldName of fieldNames) {
        const field = fields.find((item) => item.name === fieldName);
        if (!field) continue;
        const value = values[field.name] ?? "";
        if (field.type === "password" && value === "") continue;
        data[field.name] = field.type === "integer-list" ? integerList(field, value) : value;
      }
      setSaving(action);
      await runPluginActionOn(target, name, action, data);
      setValues((current) => {
        const next = { ...current };
        for (const field of fields) if (field.type === "password") next[field.name] = "";
        return next;
      });
      await loadStatus();
      setNotice("Saved");
    } catch (cause) {
      setError(message(cause));
    } finally {
      setSaving("");
    }
  };

  if (loading) return <p className="text-sm text-muted-foreground">Loading integration…</p>;
  if (!plugin?.settings) return <p role="alert" className="text-sm text-destructive">{error || "Integration settings not found"}</p>;

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="space-y-2">
        <h2 className="text-xl font-semibold">{plugin.settings.title}</h2>
        {plugin.description && <p className="text-sm text-muted-foreground">{plugin.description}</p>}
      </div>
      {!!plugin.settings.status?.length && (
        <div className="space-y-1 rounded-lg border p-4 text-sm">
          {plugin.settings.status.map((item) => (
            <p key={item.name}>{item.label}: {statusValue(status[item.name])}</p>
          ))}
        </div>
      )}
      {plugin.settings.sections?.map((section) => {
        const fields = section.fields ?? [];
        return (
          <section key={section.title} className="space-y-4 rounded-lg border p-4">
            <h3 className="font-medium">{section.title}</h3>
            {fields.map((field) => (
              <div key={field.name} className="space-y-1.5">
                <Label htmlFor={`plugin-${name}-${field.name}`}>{field.label}</Label>
                <Input
                  id={`plugin-${name}-${field.name}`}
                  type={field.type === "password" ? "password" : "text"}
                  inputMode={field.type === "integer-list" ? "numeric" : undefined}
                  autoComplete={field.type === "password" ? "off" : undefined}
                  value={values[field.name] ?? ""}
                  onChange={(event) => setValues((current) => ({ ...current, [field.name]: event.target.value }))}
                />
                {field.help && <p className="text-xs text-muted-foreground">{field.help}</p>}
              </div>
            ))}
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            <p role="status" className="text-sm text-muted-foreground">{notice}</p>
            <div className="flex gap-2">
              {section.actions?.map((button) => (
                <Button
                  key={button.action}
                  type="button"
                  disabled={saving !== ""}
                  onClick={() => void act(button.action, button.fields ?? [], fields)}
                >
                  {saving === button.action ? `${button.label}…` : button.label}
                </Button>
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}
