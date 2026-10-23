import { useHarnessCatalog } from "@/hooks/useHarnessCatalog";
import { runtimePresetGroups } from "@/lib/runtimePresets";
import { DraftField } from "./ConfigControls";
import { fieldOfKey, type ConfigField } from "./registry";
import type { BlockProps } from "./ConfigurationBlocks";

const field = (key: string) => fieldOfKey(key) as ConfigField;

// Operator-selectable harnesses. This is intentionally a SUBSET of the backend
// allowlist (internal/harness.Get() = claude | codex | opencode | cursor | stub): 'stub'
// is a test-only harness that the compose validator rejects, so it is omitted
// here to keep operators from picking a value that can't be applied. An agent
// already on 'stub' still shows it as its current value.
const HARNESS_OPTIONS = ["claude", "codex", "opencode", "cursor"] as const;

export function RuntimeBlock({ draft, matched, sources, target }: BlockProps) {
  const view = sources.view!;
  const harness = draft.values.harness ?? view.harness;
  const catalog = useHarnessCatalog(target, harness);
  const modelPresets = runtimePresetGroups({ harness, field: "models", catalog, extras: [view.model] });
  const effortPresets = runtimePresetGroups({ harness, field: "efforts", catalog, model: draft.values.model, extras: [view.effort] });
  // The operator subset, plus the agent's current harness if it falls outside
  // that subset, so the select never shows blank for an out-of-list value.
  const harnessOptions = (HARNESS_OPTIONS as readonly string[]).includes(view.harness)
    ? HARNESS_OPTIONS
    : [...HARNESS_OPTIONS, view.harness];
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <DraftField field={field("harness")} draft={draft} matched={matched.has("harness")} options={harnessOptions} />
      <DraftField field={field("interactive")} draft={draft} matched={matched.has("interactive")} />
      <DraftField field={field("model")} draft={draft} matched={matched.has("model")} presets={modelPresets} />
      <DraftField field={field("effort")} draft={draft} matched={matched.has("effort")} presets={effortPresets} />
    </div>
  );
}
