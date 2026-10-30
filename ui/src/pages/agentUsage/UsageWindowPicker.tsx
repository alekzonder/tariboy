import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { GROUPINGS, PRESET_ORDER, type GroupBy, type Preset } from "./usageParams";

// The page's own window selector: presets plus a custom datetime-local range,
// shown only when the preset is 'custom'.
export function UsageWindowPicker({
  preset,
  onPreset,
  customSince,
  onCustomSince,
  customUntil,
  onCustomUntil,
}: {
  preset: Preset;
  onPreset: (p: Preset) => void;
  customSince: string;
  onCustomSince: (v: string) => void;
  customUntil: string;
  onCustomUntil: (v: string) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {PRESET_ORDER.map((p) => (
        <button
          key={p}
          onClick={() => onPreset(p)}
          className={cn(
            "rounded border px-3 py-1 text-sm hover:bg-accent",
            preset === p && "bg-accent font-medium",
          )}
        >
          {p === "custom" ? "Custom" : p}
        </button>
      ))}
      {preset === "custom" && (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <RangeInput label="from" value={customSince} onChange={onCustomSince} />
          <RangeInput label="to" value={customUntil} onChange={onCustomUntil} />
        </div>
      )}
    </div>
  );
}

function RangeInput({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <label className="flex items-center gap-1">
      <span className="text-muted-foreground">{label}</span>
      <Input
        type="datetime-local"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-auto w-auto rounded border bg-background px-2 py-1 text-sm dark:bg-background"
      />
    </label>
  );
}

// The grouping switch above the usage table.
export function UsageGroupBySwitch({
  groupBy,
  onGroupBy,
}: {
  groupBy: GroupBy;
  onGroupBy: (g: GroupBy) => void;
}) {
  return (
    <div className="flex items-center gap-1">
      {GROUPINGS.map((g) => (
        <button
          key={g.key}
          onClick={() => onGroupBy(g.key)}
          className={cn(
            "rounded px-3 py-1 text-sm hover:bg-accent",
            groupBy === g.key ? "bg-accent font-medium" : "text-muted-foreground",
          )}
        >
          {g.label}
        </button>
      ))}
    </div>
  );
}
