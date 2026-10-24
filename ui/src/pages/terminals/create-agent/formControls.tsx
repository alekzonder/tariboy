import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { AgentPolicy } from "../agentCreateDraft";

export function Field({
  label,
  id,
  children,
}: {
  label: string;
  id: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  );
}

export function SwitchRow({
  label,
  id,
  checked,
  onCheckedChange,
  disabled,
  help,
}: {
  label: string;
  id: string;
  checked: boolean;
  onCheckedChange: (value: boolean) => void;
  disabled: boolean;
  help: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <div>
        <Label htmlFor={id}>{label}</Label>
        <p className="text-xs text-muted-foreground">{help}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
    </div>
  );
}

export function NumberField({
  label,
  id,
  value,
  onChange,
  disabled,
  min,
}: {
  label: string;
  id: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  min: number;
}) {
  return (
    <Field label={label} id={id}>
      <Input
        id={id}
        aria-label={label}
        type="number"
        min={min}
        step={1}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        className="h-8"
      />
    </Field>
  );
}

export function PolicyField({
  label,
  id,
  value,
  onChange,
  disabled,
}: {
  label: string;
  id: string;
  value: AgentPolicy;
  onChange: (value: AgentPolicy) => void;
  disabled: boolean;
}) {
  return (
    <Field label={label} id={id}>
      <select
        id={id}
        aria-label={label}
        value={value}
        onChange={(event) => onChange(event.target.value as AgentPolicy)}
        disabled={disabled}
        className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
      >
        <option value="restart">restart</option>
        <option value="stop">stop</option>
      </select>
    </Field>
  );
}
