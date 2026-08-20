import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

export { EFFORT_PRESETS } from "@/lib/runtimePresets";

// ComboField is a labeled free-text input paired with a preset dropdown, used
// for inline model/effort editing. It commits on blur, Enter, or preset select.
// The Overview remounts it (via `key`) when the committed value changes, so a
// simple once-initialized local draft is correct — no polled-value sync needed.
export function ComboField({
  label, value, presets, onCommit,
}: { label: string; value: string; presets: readonly string[]; onCommit: (v: string) => void }) {
  const [draft, setDraft] = useState(value);

  return (
    <div className="flex flex-col gap-1">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <div className="flex items-center gap-1">
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => { if (draft !== value) onCommit(draft); }}
          onKeyDown={(e) => { if (e.key === "Enter") onCommit(draft); }}
          className="h-8 w-40"
          placeholder={label}
        />
        <Select value="" onValueChange={(v) => { setDraft(v); onCommit(v); }}>
          <SelectTrigger className="h-8 w-8 p-0" aria-label={`${label} presets`}><SelectValue /></SelectTrigger>
          <SelectContent>
            {presets.map((p) => <SelectItem key={p} value={p}>{p}</SelectItem>)}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
