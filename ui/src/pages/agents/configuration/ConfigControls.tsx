import { useContext, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { SearchInput } from "@/components/ui/search-input";
import { cn } from "@/lib/utils";
import type { PresetGroup } from "@/lib/runtimePresets";
import { DraftControl } from "./DraftControls";
import { describedBy, unsaved } from "./draftFieldModel";
import { fieldOfKey, type ConfigField } from "./registry";
import type { ConfigDraft } from "./useConfigDraft";
import type { ConfigSources } from "./useConfigSources";
import { SettingsFilterContext } from "./settingsFilterContext";

export function ConfigSearch({
  query,
  onQuery,
  empty,
}: {
  query: string;
  onQuery: (value: string) => void;
  empty: boolean;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <SearchInput
          aria-label="Search settings"
          placeholder="Search settings…"
          value={query}
          onChange={(event) => onQuery(event.target.value)}
          className="h-8 min-w-0 flex-1"
        />
        {query.trim() && (
          <Button variant="ghost" size="sm" className="h-8" onClick={() => onQuery("")}>
            Clear search
          </Button>
        )}
      </div>
      {empty && (
        <p role="status" className="text-sm text-muted-foreground">
          No settings match your search.
        </p>
      )}
    </div>
  );
}

// SectionProps is what every group and Advanced subsection receives.
export interface SectionProps {
  draft: ConfigDraft;
  sources: ConfigSources;
  name: string;
  stopped: boolean;
  refresh: () => void;
}

// SettingsSection is one plain group: a heading and a thin rule, no card, so
// the four groups fit above the fold. Advanced subsections use level 4.
export function SettingsSection({
  id,
  title,
  level = 3,
  action,
  children,
}: {
  id: string;
  title: string;
  level?: 3 | 4;
  action?: ReactNode;
  children: ReactNode;
}) {
  const visible = useContext(SettingsFilterContext);
  const hidden = visible !== null && !visible.has(id);
  const Heading = level === 3 ? "h3" : "h4";
  return (
    <section
      id={`settings-section-${id}`}
      hidden={hidden}
      aria-labelledby={`settings-section-${id}-title`}
      className={
        hidden
          ? "hidden"
          : cn(
              "scroll-mt-24 border-t pt-3",
              level === 3 && "grid gap-x-6 gap-y-2 sm:grid-cols-[9rem_minmax(0,1fr)]",
            )
      }
    >
      <div className={cn("flex items-center gap-2", level === 4 && "mb-3")}>
        <Heading
          id={`settings-section-${id}-title`}
          className={level === 3 ? "text-sm font-semibold" : "text-sm font-medium"}
        >
          {title}
        </Heading>
        {action}
      </div>
      <div className="min-w-0 space-y-3">{children}</div>
    </section>
  );
}

// FieldFrame is the shared label / changed marker / error / helper layout, so
// every setting on the page reads the same and a failed save is announced
// beside the control it belongs to.
export function FieldFrame({
  field,
  draft,
  inline,
  children,
}: {
  field: ConfigField;
  draft: ConfigDraft;
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
    <div data-field={field.key} className="space-y-1.5">
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

// DraftField renders one plain setting bound to the page draft.
export function DraftField({
  field,
  draft,
  options,
  presets,
  disabled,
}: {
  field: ConfigField;
  draft: ConfigDraft;
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
      <FieldFrame field={field} draft={draft} inline>
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
    <FieldFrame field={field} draft={draft}>
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

// ConfigSaveRow is the page title and its only commit point. It sticks to the
// top of the tab, so "Save changes" stays reachable at the end of Advanced.
export function ConfigSaveRow({ draft, groups }: { draft: ConfigDraft; groups: string[] }) {
  const count = draft.dirtyKeys.length;
  const status = count > 0 ? `${unsaved(count)} in ${groups.join(", ")}` : draft.notice;
  return (
    <div
      role="region"
      aria-label="Save settings"
      className="sticky top-0 z-10 -mx-4 -mt-4 border-b bg-card/95 px-4 py-2.5 backdrop-blur supports-[backdrop-filter]:bg-card/80"
    >
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="font-semibold">Settings</h2>
        <div className="min-w-0 flex-1 space-y-0.5">
          <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
            {status}
          </p>
          {draft.error && (
            <p role="alert" className="text-xs text-destructive">
              {draft.error}
            </p>
          )}
        </div>
        {count > 0 && (
          <>
            <Button variant="outline" className="h-8" disabled={draft.saving} onClick={draft.reset}>
              Reset
            </Button>
            <Button className="h-8" disabled={draft.saving} onClick={() => void draft.save()}>
              {draft.saving ? "Saving…" : "Save changes"}
            </Button>
          </>
        )}
      </div>
    </div>
  );
}

export function ReadOnly({
  draft,
  fieldKey,
  children,
}: {
  draft: ConfigDraft;
  fieldKey: string;
  children: React.ReactNode;
}) {
  return (
    <FieldFrame field={fieldOfKey(fieldKey) as ConfigField} draft={draft}>
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
