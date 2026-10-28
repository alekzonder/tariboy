import type { ComponentType } from "react";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { EditablePresetCombobox } from "@/components/EditablePresetCombobox";
import type { PresetGroup } from "@/lib/runtimePresets";
import { inputAttributes } from "./draftFieldModel";
import type { ConfigField } from "./registry";
import { ScheduleControl } from "./ScheduleControl";

// Mirror the shadcn Input look (rounded-lg, same border + focus ring) so
// selects and text inputs line up as one visual family, in light and dark.
export const selectClass =
  "h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none " +
  "transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 " +
  "dark:bg-input/30";

// ControlProps is what every plain setting control receives from DraftField.
export interface ControlProps {
  field: ConfigField;
  value: string;
  set: (next: string) => void;
  common: {
    id: string;
    "aria-describedby": string | undefined;
    "aria-invalid": boolean | undefined;
  };
  unloaded: boolean;
  options?: readonly string[];
  presets?: readonly PresetGroup[];
}

function PresetControl({ field, value, set, common, presets }: ControlProps) {
  return (
    <EditablePresetCombobox
      id={field.key}
      ariaLabel={field.label}
      value={value}
      groups={presets ?? []}
      describedBy={common["aria-describedby"]}
      invalid={Boolean(common["aria-invalid"])}
      onChange={set}
      placeholder="image default"
    />
  );
}

function SelectControl({ field, value, set, common, unloaded, options }: ControlProps) {
  const choices = options ?? field.options ?? [];
  return (
    <select
      {...common}
      value={value}
      disabled={unloaded}
      className={selectClass}
      onChange={(e) => set(e.target.value)}
    >
      {choices.map((o) => (
        <option key={o} value={o}>
          {o}
        </option>
      ))}
    </select>
  );
}

function TextareaControl({ value, set, common, unloaded }: ControlProps) {
  return (
    <Textarea
      {...common}
      value={value}
      disabled={unloaded}
      className="min-h-40 font-mono"
      onChange={(e) => set(e.target.value)}
    />
  );
}

function InputControl({ field, value, set, common, unloaded }: ControlProps) {
  const { type, autoComplete, min, step } = inputAttributes(field);
  return (
    <Input
      {...common}
      value={value}
      disabled={unloaded}
      className="h-9"
      type={type}
      autoComplete={autoComplete}
      min={min}
      step={step}
      onChange={(e) => set(e.target.value)}
    />
  );
}

const CONTROLS: Partial<Record<ConfigField["kind"], ComponentType<ControlProps>>> = {
  preset: PresetControl,
  select: SelectControl,
  textarea: TextareaControl,
  schedule: ScheduleControl,
};

// DraftControl renders the control a field kind maps to; every other kind is
// a plain input.
export function DraftControl(props: ControlProps) {
  const Control = CONTROLS[props.field.kind] ?? InputControl;
  return <Control {...props} />;
}
