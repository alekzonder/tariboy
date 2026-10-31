import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { useApi } from "@/components/apiContext";
import { useHarnessCatalog } from "@/hooks/useHarnessCatalog";
import { runtimePresetGroups } from "@/lib/runtimePresets";
import { autopilotMode, rememberedMinutes, toMinutes, toSeconds } from "./autopilotModel";
import { DraftField, FieldFrame, SettingsSection, type SectionProps } from "./ConfigControls";
import { describedBy } from "./draftFieldModel";
import { fieldOfKey, type ConfigField } from "./registry";
import type { ConfigDraft } from "./useConfigDraft";

const field = (key: string) => fieldOfKey(key) as ConfigField;

// Operator-selectable harnesses. This is intentionally a SUBSET of the backend
// allowlist (internal/harness.Get() = claude | codex | opencode | cursor | stub): 'stub'
// is a test-only harness that the compose validator rejects, so it is omitted
// here to keep operators from picking a value that can't be applied. An agent
// already on 'stub' still shows it as its current value.
const HARNESS_OPTIONS = ["claude", "codex", "opencode", "cursor"] as const;

function ModelGroup({ draft, sources }: SectionProps) {
  const api = useApi();
  const view = sources.view!;
  const harness = draft.values.harness ?? view.harness;
  const catalog = useHarnessCatalog(api, harness);
  const modelPresets = runtimePresetGroups({
    harness,
    field: "models",
    catalog,
    extras: [view.model],
  });
  const effortPresets = runtimePresetGroups({
    harness,
    field: "efforts",
    catalog,
    model: draft.values.model,
    extras: [view.effort],
  });
  // The operator subset, plus the agent's current harness if it falls outside
  // that subset, so the select never shows blank for an out-of-list value.
  const harnessOptions = (HARNESS_OPTIONS as readonly string[]).includes(view.harness)
    ? HARNESS_OPTIONS
    : [...HARNESS_OPTIONS, view.harness];
  return (
    <SettingsSection id="model" title="Model">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <DraftField field={field("harness")} draft={draft} options={harnessOptions} />
        <DraftField field={field("model")} draft={draft} presets={modelPresets} />
        <DraftField field={field("effort")} draft={draft} presets={effortPresets} />
      </div>
      <p className="text-xs text-muted-foreground">Takes effect on the next run.</p>
    </SettingsSection>
  );
}

// The working directory can change only while the agent is stopped. The
// editor opens in place on "Change…" and stays open while it holds a draft.
function FolderGroup({
  draft,
  sources,
  stopped,
  editing,
  onEdit,
}: SectionProps & { editing: boolean; onEdit: () => void }) {
  const cwd = sources.view?.cwd;
  const cwdError = draft.fieldErrors.cwd ?? "";
  const open =
    stopped &&
    (editing || draft.dirtyKeys.includes("cwd") || draft.dirtyKeys.includes("cwd-create"));
  return (
    <SettingsSection id="folder" title="Working folder">
      {open ? (
        <>
          <FieldFrame field={field("cwd")} draft={draft}>
            <PathAutocomplete
              id="cwd"
              value={draft.values.cwd ?? ""}
              onChange={(value) => draft.setField("cwd", value)}
              aria-label="Working directory"
            />
          </FieldFrame>
          <DraftField field={field("cwd-create")} draft={draft} />
        </>
      ) : (
        <div data-field="cwd" className="space-y-1 text-sm">
          <div className="flex flex-wrap items-center gap-3">
            <p className="min-w-0 font-mono text-xs break-all">{cwd || "—"}</p>
            <Button
              size="sm"
              variant="outline"
              className="h-7"
              disabled={!stopped}
              onClick={onEdit}
            >
              Change…
            </Button>
          </div>
          {cwdError && (
            <p role="alert" className="text-xs text-destructive">
              {cwdError}
            </p>
          )}
          {!stopped && (
            <p className="text-xs text-muted-foreground">
              Stop the agent before changing its working directory.
            </p>
          )}
        </div>
      )}
    </SettingsSection>
  );
}

// MinutesInput edits a seconds draft in minutes. It keeps the typed text, so
// "1." or an emptied box is not rewritten while typing, and follows the draft
// again when it changes from outside (Reset, a reload).
function MinutesInput({
  fieldKey,
  draft,
  text,
  setText,
  disabled,
  className,
}: {
  fieldKey: string;
  draft: ConfigDraft;
  text: string;
  setText: (text: string) => void;
  disabled?: boolean;
  className?: string;
}) {
  const f = field(fieldKey);
  return (
    <Input
      id={fieldKey}
      aria-label={f.label}
      aria-describedby={describedBy(f, draft)}
      aria-invalid={draft.fieldErrors[fieldKey] ? true : undefined}
      inputMode="decimal"
      className={className ?? "h-8 w-20"}
      value={text}
      disabled={disabled || !(fieldKey in draft.values)}
      onChange={(e) => {
        setText(e.target.value);
        draft.setField(fieldKey, toSeconds(e.target.value));
      }}
    />
  );
}

// useMinutesText holds a minutes box's text over a seconds draft value.
function useMinutesText(seconds: string, initial: string, follow: boolean) {
  const [text, setText] = useState(initial);
  if (follow && toSeconds(text) !== seconds) {
    const next = toMinutes(seconds);
    if (next !== text) setText(next);
  }
  return [text, setText] as const;
}

function FieldMessages({ fieldKey, draft }: { fieldKey: string; draft: ConfigDraft }) {
  const error = draft.fieldErrors[fieldKey];
  const helper = field(fieldKey).helper;
  return (
    <>
      {error && (
        <p id={`${fieldKey}-error`} role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      {helper && (
        <p id={`${fieldKey}-help`} className="text-xs text-muted-foreground">
          {helper}
        </p>
      )}
    </>
  );
}

function AutopilotGroup({ draft, sources }: SectionProps) {
  const interval = draft.values.interval ?? "";
  const timeout = draft.values.timeout ?? "";
  const mode = autopilotMode(interval);
  const [every, setEvery] = useMinutesText(interval, rememberedMinutes(interval), mode === "every");
  const [limit, setLimit] = useMinutesText(timeout, toMinutes(timeout), true);
  const changed = (key: string) =>
    draft.dirtyKeys.includes(key) && (
      <span className="text-xs font-medium text-primary">Changed</span>
    );
  return (
    <SettingsSection id="autopilot" title="Autopilot">
      <div role="radiogroup" aria-label="When the agent runs" className="space-y-1.5 text-sm">
        <label className="flex items-center gap-2">
          <input
            type="radio"
            name="autopilot-mode"
            checked={mode === "on-demand"}
            disabled={!("interval" in draft.values)}
            onChange={() => draft.setField("interval", "0")}
          />
          Only when given tasks or messages
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-2">
            <input
              type="radio"
              name="autopilot-mode"
              checked={mode === "every"}
              disabled={!("interval" in draft.values)}
              onChange={() =>
                draft.setField(
                  "interval",
                  toSeconds(Number(every) > 0 ? every : rememberedMinutes(interval)),
                )
              }
            />
            Also every
          </label>
          <MinutesInput
            fieldKey="interval"
            draft={draft}
            text={every}
            setText={setEvery}
            disabled={mode !== "every"}
          />
          <span>minutes</span>
          {changed("interval")}
        </div>
        <FieldMessages fieldKey="interval" draft={draft} />
        {sources.view?.schedule && (
          <p className="text-xs text-muted-foreground">
            A schedule also starts runs: see Advanced → Schedule and timeouts.
          </p>
        )}
      </div>
      <div className="space-y-1.5 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <label htmlFor="timeout">Stop a run after</label>
          <MinutesInput fieldKey="timeout" draft={draft} text={limit} setText={setLimit} />
          <span>min</span>
          {changed("timeout")}
        </div>
        <FieldMessages fieldKey="timeout" draft={draft} />
      </div>
      <DraftField field={field("interactive")} draft={draft} />
    </SettingsSection>
  );
}

function SpendingGroup({ draft, sources }: SectionProps) {
  const budget = sources.view?.budget;
  if (!budget) return null;
  const exhausted = budget.exhausted ?? [];
  const periods = [
    ["day", "today"],
    ["month", "this month"],
  ] as const;
  return (
    <SettingsSection id="spending" title="Spending limit">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {periods.map(([period, when]) => {
          const key = `budget-${period}`;
          const spent = budget[`${period}_spent_usd`];
          const helper = `USD (0 = none) · spent $${spent.toFixed(2)} ${when}`;
          return <DraftField key={key} field={{ ...field(key), helper }} draft={draft} />;
        })}
      </div>
      {exhausted.length > 0 && (
        <p role="status" className="text-sm text-destructive">
          Out of budget: {exhausted.join(", ")}
        </p>
      )}
    </SettingsSection>
  );
}

// SettingsGroups is the top of the page: the four groups an operator needs.
export function SettingsGroups(
  props: SectionProps & { editingFolder: boolean; onEditFolder: () => void },
) {
  return (
    <>
      <ModelGroup {...props} />
      <FolderGroup {...props} editing={props.editingFolder} onEdit={props.onEditFolder} />
      <AutopilotGroup {...props} />
      <SpendingGroup {...props} />
    </>
  );
}
