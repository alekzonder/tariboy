import { useId, useState } from "react";
import { Input } from "@/components/ui/input";
import type { PresetGroup } from "@/lib/runtimePresets";

export function EditablePresetCombobox({
  id,
  ariaLabel,
  value,
  options = [],
  groups,
  onChange,
  placeholder,
  disabled = false,
  describedBy,
  invalid = false,
}: {
  id?: string;
  ariaLabel: string;
  value: string;
  options?: readonly string[];
  // Labeled sections; a group with a note keeps its header to show the note
  // (loading, an error) even when it has no options.
  groups?: readonly PresetGroup[];
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  describedBy?: string;
  invalid?: boolean;
}) {
  const generatedId = useId();
  const listboxId = `${id ?? generatedId}-presets`;
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const normalizedFilter = filter.toLowerCase();
  const visibleGroups = (groups ?? [{ label: "", options: [...options] }])
    .map((group) => ({
      ...group,
      options: group.options.filter((option) =>
        option.toLowerCase().includes(normalizedFilter),
      ),
    }))
    .filter((group) => group.options.length > 0 || group.note);

  const renderOption = (option: string) => (
    <button
      key={option}
      type="button"
      role="option"
      aria-selected={option === value}
      onMouseDown={(event) => event.preventDefault()}
      onClick={() => {
        onChange(option);
        setFilter("");
        setOpen(false);
      }}
      className="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-accent"
    >
      {option}
    </button>
  );

  return (
    <div className="relative">
      <Input
        id={id}
        role="combobox"
        aria-label={ariaLabel}
        aria-autocomplete="list"
        aria-controls={listboxId}
        aria-expanded={open}
        aria-describedby={describedBy}
        aria-invalid={invalid ? true : undefined}
        value={value}
        onChange={(event) => {
          const next = event.target.value;
          onChange(next);
          setFilter(next);
          setOpen(true);
        }}
        onFocus={() => {
          if (disabled) return;
          setFilter("");
          setOpen(true);
        }}
        onBlur={() => setOpen(false)}
        placeholder={placeholder}
        disabled={disabled}
        className="h-8"
      />
      {open && (
        <div
          id={listboxId}
          role="listbox"
          className="absolute top-full left-0 z-50 mt-1 max-h-64 w-full overflow-auto rounded-lg border bg-popover p-1 shadow-md"
        >
          {visibleGroups.length === 0 && (
            <div className="px-2 py-2 text-sm text-muted-foreground">No presets</div>
          )}
          {visibleGroups.map((group) => {
            if (!group.label) return group.options.map(renderOption);
            const headerId = `${listboxId}-${group.label.replace(/\W+/g, "-")}`;
            return (
              <div key={group.label} role="group" aria-labelledby={headerId}>
                <div
                  id={headerId}
                  className="px-2 pt-1.5 pb-1 text-xs font-medium text-muted-foreground"
                >
                  {group.label}
                </div>
                {group.note && (
                  <div className="px-2 pb-1.5 text-xs text-muted-foreground">
                    {group.note}
                  </div>
                )}
                {group.options.map(renderOption)}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
