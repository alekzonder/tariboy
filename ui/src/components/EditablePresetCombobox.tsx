import { useId, useState } from "react";
import { Input } from "@/components/ui/input";

export function EditablePresetCombobox({
  id,
  ariaLabel,
  value,
  options,
  onChange,
  placeholder,
  disabled = false,
}: {
  id?: string;
  ariaLabel: string;
  value: string;
  options: readonly string[];
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
}) {
  const generatedId = useId();
  const listboxId = `${id ?? generatedId}-presets`;
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const normalizedFilter = filter.toLowerCase();
  const filtered = options.filter((option) =>
    option.toLowerCase().includes(normalizedFilter),
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
          {filtered.length === 0 && (
            <div className="px-2 py-2 text-sm text-muted-foreground">No presets</div>
          )}
          {filtered.map((option) => (
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
          ))}
        </div>
      )}
    </div>
  );
}
