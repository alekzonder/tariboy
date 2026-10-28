import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import type { ControlProps } from "./DraftControls";
import { selectClass } from "./DraftControls";
import {
  MODES,
  WEEKDAYS,
  describeSchedule,
  formatSchedule,
  parseSchedule,
  switchMode,
  type ScheduleMode,
  type ScheduleModel,
} from "./scheduleModel";

const MONTH_DAYS = [
  ...Array.from({ length: 31 }, (_, i) => ({ value: String(i + 1), label: String(i + 1) })),
  { value: "-1", label: "Last day" },
];

interface ModeProps {
  model: ScheduleModel;
  update: (patch: Partial<ScheduleModel>) => void;
  disabled: boolean;
}

function TimeInput({ model, update, disabled }: ModeProps) {
  return (
    <Input
      aria-label="Time"
      type="time"
      value={model.time}
      disabled={disabled}
      className="h-9 w-32"
      // An emptied time input keeps the last time instead of breaking the rule.
      onChange={(e) => e.target.value && update({ time: e.target.value })}
    />
  );
}

function EveryFields({ model, update, disabled }: ModeProps) {
  return (
    <div className="flex items-center gap-2">
      <Input
        aria-label="Repeat every"
        type="number"
        min={1}
        step={1}
        value={model.every}
        disabled={disabled}
        className="h-9 w-24"
        onChange={(e) => update({ every: e.target.value })}
      />
      <select
        aria-label="Unit"
        value={model.unit}
        disabled={disabled}
        className={cn(selectClass, "w-32")}
        onChange={(e) => update({ unit: e.target.value as ScheduleModel["unit"] })}
      >
        <option value="MINUTELY">minutes</option>
        <option value="HOURLY">hours</option>
      </select>
    </div>
  );
}

function WeekdayToggles({ model, update, disabled }: ModeProps) {
  const toggle = (code: string) =>
    update({
      days: model.days.includes(code)
        ? model.days.filter((d) => d !== code)
        : [...model.days, code],
    });
  return (
    <div className="flex flex-wrap gap-1">
      {WEEKDAYS.map(([code, name]) => {
        const on = model.days.includes(code);
        return (
          <button
            key={code}
            type="button"
            aria-pressed={on}
            // A weekly rule needs at least one day.
            disabled={disabled || (on && model.days.length === 1)}
            onClick={() => toggle(code)}
            className={cn(
              "h-9 w-11 rounded-lg border border-input text-sm transition-colors",
              on ? "border-primary bg-primary text-primary-foreground" : "bg-background",
            )}
          >
            {name}
          </button>
        );
      })}
    </div>
  );
}

function ModeFields(props: ModeProps) {
  const { model, update, disabled } = props;
  switch (model.mode) {
    case "every":
      return <EveryFields {...props} />;
    case "daily":
      return <TimeInput {...props} />;
    case "weekly":
      return (
        <div className="space-y-2">
          <WeekdayToggles {...props} />
          <TimeInput {...props} />
        </div>
      );
    case "monthly":
      return (
        <div className="flex items-center gap-2">
          <select
            aria-label="Day of month"
            value={model.monthDay}
            disabled={disabled}
            className={cn(selectClass, "w-32")}
            onChange={(e) => update({ monthDay: e.target.value })}
          >
            {MONTH_DAYS.map((d) => (
              <option key={d.value} value={d.value}>
                {d.label}
              </option>
            ))}
          </select>
          <TimeInput {...props} />
        </div>
      );
    case "once":
      return (
        <Input
          aria-label="Date and time"
          type="datetime-local"
          value={model.date}
          disabled={disabled}
          className="h-9 w-56"
          onChange={(e) => e.target.value && update({ date: e.target.value })}
        />
      );
    case "custom":
      return (
        <Textarea
          aria-label="Custom RRULE"
          value={model.custom}
          disabled={disabled}
          className="min-h-24 font-mono"
          onChange={(e) => update({ custom: e.target.value })}
        />
      );
    default:
      return null;
  }
}

// ScheduleControl edits the schedule draft through a mode and its fields. The
// draft keeps the RRULE text; the builder state is rebuilt from it whenever the
// text changes from outside (load, Save, Reset).
export function ScheduleControl({ value, set, common, unloaded }: ControlProps) {
  const [model, setModel] = useState(() => parseSchedule(value));
  const [seen, setSeen] = useState(value);
  if (value !== seen) {
    setSeen(value);
    setModel(parseSchedule(value));
  }
  const apply = (next: ScheduleModel) => {
    const text = formatSchedule(next);
    setModel(next);
    setSeen(text);
    set(text);
  };
  const built = model.mode !== "none" && model.mode !== "custom";
  return (
    <div className="space-y-2">
      <select
        {...common}
        value={model.mode}
        disabled={unloaded}
        className={cn(selectClass, "sm:w-64")}
        onChange={(e) => apply(switchMode(model, e.target.value as ScheduleMode, new Date()))}
      >
        {MODES.map((m) => (
          <option key={m.value} value={m.value}>
            {m.label}
          </option>
        ))}
      </select>
      <ModeFields
        model={model}
        update={(patch) => apply({ ...model, ...patch })}
        disabled={unloaded}
      />
      {built && (
        <div className="space-y-0.5 text-xs">
          <p data-testid="schedule-summary">{describeSchedule(model)}</p>
          <code
            data-testid="schedule-rrule"
            className="block whitespace-pre-wrap text-muted-foreground"
          >
            {formatSchedule(model)}
          </code>
        </div>
      )}
    </div>
  );
}
