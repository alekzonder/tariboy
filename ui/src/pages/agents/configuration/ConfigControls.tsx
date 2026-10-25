import type { ReactNode } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import type { PresetGroup } from "@/lib/runtimePresets";
import { DraftControl } from "./DraftControls";
import { fieldOfKey, type ConfigBlock, type ConfigField } from "./registry";
import type { ConfigDraft } from "./useConfigDraft";

const unsaved = (count: number) => `${count} unsaved ${count === 1 ? "change" : "changes"}`;

export function ConfigBlockCard({
  block,
  dirtyCount,
  action,
  children,
}: {
  block: ConfigBlock;
  dirtyCount: number;
  action?: ReactNode;
  children: ReactNode;
}) {
  // overflow-visible: Card clips by default, which hid the in-place pop-up lists
  // (preset combobox, path suggestions) behind the card's bottom edge.
  return (
    <Card
      id={`config-block-${block.id}`}
      role="region"
      aria-labelledby={`config-block-${block.id}-title`}
      className="scroll-mt-32 overflow-visible"
    >
      <CardHeader>
        <CardTitle id={`config-block-${block.id}-title`} className="text-base">
          {block.title}
        </CardTitle>
        <CardDescription>{block.description}</CardDescription>
        {(dirtyCount > 0 || action) && (
          <CardAction className="flex items-center gap-2">
            {dirtyCount > 0 && <Badge variant="secondary">{unsaved(dirtyCount)}</Badge>}
            {action}
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="space-y-5">{children}</CardContent>
    </Card>
  );
}

// FieldFrame is the shared label / changed marker / error / helper layout, so
// every setting on the page reads the same and a failed save is announced
// beside the control it belongs to.
export function FieldFrame({
  field,
  draft,
  matched,
  inline,
  children,
}: {
  field: ConfigField;
  draft: ConfigDraft;
  matched: boolean;
  inline?: boolean;
  children: ReactNode;
}) {
  const error = draft.fieldErrors[field.key] ?? "";
  const changed = draft.dirtyKeys.includes(field.key);
  const label = (
    <div className="flex items-center gap-2">
      <Label htmlFor={field.key} className={inline ? "cursor-pointer" : undefined}>
        {field.label}
      </Label>
      {changed && <span className="text-xs font-medium text-primary">Changed</span>}
    </div>
  );
  return (
    <div
      data-field={field.key}
      data-matched={matched || undefined}
      className={cn(
        "space-y-1.5 rounded-lg",
        matched && "-m-2 bg-primary/5 p-2 ring-1 ring-primary/30",
      )}
    >
      {inline ? (
        <div className="flex items-center gap-3">
          {children}
          {label}
        </div>
      ) : (
        <>
          {label}
          {children}
        </>
      )}
      {error && (
        <p id={`${field.key}-error`} role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      {field.helper && (
        <p id={`${field.key}-help`} className="text-xs text-muted-foreground">
          {field.helper}
        </p>
      )}
    </div>
  );
}

const describedBy = (field: ConfigField, draft: ConfigDraft) =>
  [field.helper && `${field.key}-help`, draft.fieldErrors[field.key] && `${field.key}-error`]
    .filter(Boolean)
    .join(" ") || undefined;

// DraftField renders one plain setting bound to the page draft.
export function DraftField({
  field,
  draft,
  matched,
  options,
  presets,
  disabled,
}: {
  field: ConfigField;
  draft: ConfigDraft;
  matched: boolean;
  options?: readonly string[];
  presets?: readonly PresetGroup[];
  disabled?: boolean;
}) {
  const value = draft.values[field.key] ?? "";
  const invalid = draft.fieldErrors[field.key] ? true : undefined;
  const common = {
    id: field.key,
    "aria-describedby": describedBy(field, draft),
    "aria-invalid": invalid,
  };
  const set = (next: string) => draft.setField(field.key, next);
  const unloaded = disabled || !(field.key in draft.values);
  if (field.kind === "switch") {
    return (
      <FieldFrame field={field} draft={draft} matched={matched} inline>
        <Switch
          {...common}
          checked={value === "true"}
          disabled={unloaded}
          onCheckedChange={(checked) => set(String(checked))}
        />
      </FieldFrame>
    );
  }
  return (
    <FieldFrame field={field} draft={draft} matched={matched}>
      <DraftControl
        field={field}
        value={value}
        set={set}
        common={common}
        unloaded={unloaded}
        options={options}
        presets={presets}
      />
    </FieldFrame>
  );
}

export function ConfigSearch({
  query,
  onQuery,
  changedOnly,
  onChangedOnly,
  dirtyCount,
  chips,
  resultText,
}: {
  query: string;
  onQuery: (query: string) => void;
  changedOnly: boolean;
  onChangedOnly: (value: boolean) => void;
  dirtyCount: number;
  chips: Array<{ id: string; title: string; dirty: number }>;
  resultText: string;
}) {
  return (
    <div className="sticky top-0 z-10 flex flex-col gap-2 bg-card/95 py-3 backdrop-blur supports-[backdrop-filter]:bg-card/80">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          type="search"
          aria-label="Search settings"
          placeholder="Search settings"
          value={query}
          onChange={(e) => onQuery(e.target.value)}
          className="h-9 min-w-0 flex-1 basis-60"
        />
        {query && (
          <Button variant="ghost" className="h-9" onClick={() => onQuery("")}>
            Clear
          </Button>
        )}
        <Button
          variant={changedOnly ? "secondary" : "outline"}
          className="h-9"
          aria-pressed={changedOnly}
          onClick={() => onChangedOnly(!changedOnly)}
        >
          Changed only ({dirtyCount})
        </Button>
      </div>
      <nav aria-label="Configuration blocks" className="flex flex-wrap gap-1.5">
        {chips.map((chip) => (
          <Button
            key={chip.id}
            size="sm"
            variant="outline"
            className="h-7 rounded-full"
            onClick={() =>
              document
                .getElementById(`config-block-${chip.id}`)
                ?.scrollIntoView?.({ block: "start", behavior: "smooth" })
            }
          >
            {chip.title}
            {chip.dirty > 0 && <Badge className="ml-1">{chip.dirty}</Badge>}
          </Button>
        ))}
      </nav>
      <p
        role="status"
        aria-live="polite"
        className={resultText ? "text-xs text-muted-foreground" : "sr-only"}
      >
        {resultText}
      </p>
    </div>
  );
}

// ConfigSaveBar is the page's only commit point. It appears once there is
// something to commit, a reason the last save stopped, or a result to report.
export function ConfigSaveBar({
  draft,
  blockTitles,
}: {
  draft: ConfigDraft;
  blockTitles: string[];
}) {
  const count = draft.dirtyKeys.length;
  if (count === 0 && !draft.error && !draft.notice) return null;
  return (
    <div
      role="region"
      aria-label="Unsaved changes"
      className="sticky bottom-0 z-10 rounded-lg border bg-card/95 p-3 shadow-md backdrop-blur supports-[backdrop-filter]:bg-card/80"
    >
      <div className="flex flex-wrap items-center gap-3">
        <div className="min-w-0 flex-1 space-y-0.5">
          <p role="status" aria-live="polite" className="text-sm font-medium">
            {count > 0 ? `${unsaved(count)} in ${blockTitles.join(", ")}` : draft.notice}
          </p>
          {draft.error && (
            <p role="alert" className="text-xs text-destructive">
              {draft.error}
            </p>
          )}
        </div>
        {count > 0 && (
          <>
            <Button variant="outline" className="h-9" disabled={draft.saving} onClick={draft.reset}>
              Reset
            </Button>
            <Button className="h-9" disabled={draft.saving} onClick={() => void draft.save()}>
              {draft.saving ? "Saving…" : "Save"}
            </Button>
          </>
        )}
      </div>
    </div>
  );
}

export function ReadOnly({
  draft,
  matched,
  fieldKey,
  children,
}: {
  draft: ConfigDraft;
  matched: Set<string>;
  fieldKey: string;
  children: React.ReactNode;
}) {
  return (
    <FieldFrame
      field={fieldOfKey(fieldKey) as ConfigField}
      draft={draft}
      matched={matched.has(fieldKey)}
    >
      {children}
    </FieldFrame>
  );
}

// Actions run at once and are never part of Save; the row says so.
export function ActionRow({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2 border-t pt-4">
      <span className="text-xs text-muted-foreground">
        Actions apply immediately, not part of Save:
      </span>
      {children}
    </div>
  );
}
