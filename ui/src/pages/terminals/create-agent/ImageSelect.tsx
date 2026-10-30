import { useState } from "react";
import { Input } from "@/components/ui/input";
import type { ImageChoice } from "@/lib/onboarding/firstRun";

function ChoiceDetail({ choice }: { choice: ImageChoice }) {
  const detail = choice.waiting
    ? choice.waiting
    : [choice.version, choice.source && `from ${choice.source}`].filter(Boolean).join(" · ");
  return detail ? <span className="text-xs text-muted-foreground">{detail}</span> : null;
}

/** One row per image name; typing filters by name. The value stays `name:tag`. */
export function ImageSelect({
  id,
  choices,
  value,
  onChange,
  disabled,
}: {
  id: string;
  choices: ImageChoice[];
  value: ImageChoice | null;
  onChange: (ref: string) => void;
  disabled: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const filtered = choices.filter((choice) =>
    choice.label.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <div className="relative">
      <Input
        id={id}
        value={open ? query : (value?.label ?? "")}
        onChange={(event) => {
          setQuery(event.target.value);
          setOpen(true);
        }}
        onFocus={() => {
          setQuery("");
          setOpen(true);
        }}
        onBlur={() => setOpen(false)}
        placeholder="Choose an image"
        aria-label="image"
        disabled={disabled}
        className="h-8"
      />
      {value && !open && (
        <div className="mt-0.5">
          <ChoiceDetail choice={value} />
        </div>
      )}
      {open && (
        <div
          role="listbox"
          className="absolute top-full left-0 z-50 mt-1 max-h-64 w-full overflow-auto rounded-lg border bg-popover p-1 shadow-md"
        >
          {filtered.length === 0 && (
            <div className="px-2 py-2 text-sm text-muted-foreground">No images</div>
          )}
          {filtered.map((choice) => (
            <button
              key={choice.name}
              type="button"
              role="option"
              aria-label={choice.label}
              aria-selected={choice.name === value?.name}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => {
                onChange(choice.ref);
                setOpen(false);
              }}
              className="flex w-full items-baseline justify-between gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent"
            >
              <span>{choice.label}</span>
              <ChoiceDetail choice={choice} />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
