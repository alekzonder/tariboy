import { readKey, storageKeys, writeKey } from "./storage";
import type { HarnessModelCatalog } from "@/lib/api";

export type RuntimePresetField = "models" | "efforts";

export interface PresetGroup {
  label: string;
  options: string[];
  note?: string;
}

export interface HarnessCatalogState {
  loading: boolean;
  catalog: HarnessModelCatalog | null;
  error: string;
}

export const RUNTIME_PRESETS_STORAGE_KEY = storageKeys.runtimePresets.name;
export const EFFORT_PRESETS = ["low", "medium", "high", "xhigh", "max", "ultracode"] as const;

const LEARNED_PRESET_LIMIT = 20;

type LearnedHarnessPresets = Partial<Record<RuntimePresetField, string[]>>;
type LearnedRuntimePresets = Record<string, LearnedHarnessPresets>;

function stringValues(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((entry): entry is string => typeof entry === "string")
    .map((entry) => entry.trim())
    .filter(Boolean)
    .slice(-LEARNED_PRESET_LIMIT);
}

function loadLearnedPresets(): LearnedRuntimePresets {
  const parsed = readKey(storageKeys.runtimePresets);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};

  const learned: LearnedRuntimePresets = {};
  for (const [harness, value] of Object.entries(parsed)) {
    if (!value || typeof value !== "object" || Array.isArray(value)) continue;
    const entry = value as Record<string, unknown>;
    const models = stringValues(entry.models);
    const efforts = stringValues(entry.efforts);
    if (models.length || efforts.length) learned[harness] = { models, efforts };
  }
  return learned;
}

function uniqueTrimmed(groups: readonly (readonly (string | undefined)[])[]): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const group of groups) {
    for (const raw of group) {
      const value = raw?.trim();
      if (!value || seen.has(value)) continue;
      seen.add(value);
      result.push(value);
    }
  }
  return result;
}

export function runtimePresetOptions(
  harness: string,
  field: RuntimePresetField,
  extras: readonly (string | undefined)[] = [],
): string[] {
  const learned = loadLearnedPresets()[harness]?.[field] ?? [];
  return uniqueTrimmed([learned, extras]);
}

export function rememberRuntimePreset(
  harness: string,
  field: RuntimePresetField,
  rawValue: string,
): void {
  const value = rawValue.trim();
  if (!harness || !value) return;

  const learned = loadLearnedPresets();
  const current = learned[harness]?.[field] ?? [];
  const next = [...current.filter((entry) => entry !== value), value].slice(-LEARNED_PRESET_LIMIT);
  learned[harness] = {
    ...learned[harness],
    [field]: next,
  };

  // Presets are a convenience. Storage failures must not block creation.
  writeKey(storageKeys.runtimePresets, learned);
}

function catalogValues(
  catalog: HarnessModelCatalog,
  field: RuntimePresetField,
  model: string,
): string[] {
  const models = catalog.models ?? [];
  if (field === "models") return models.map((entry) => entry.id);
  const selected = models.find((entry) => entry.id === model.trim());
  return selected?.efforts?.length ? selected.efforts : (catalog.efforts ?? []);
}

// Suggestions for the model and effort fields, split into what the harness CLI
// on the selected host reports and what the operator saved earlier. Saved
// values the harness already reports are listed once, under the harness.
export function runtimePresetGroups({
  harness,
  field,
  catalog,
  model = "",
  extras = [],
}: {
  harness: string;
  field: RuntimePresetField;
  catalog: HarnessCatalogState;
  model?: string;
  extras?: readonly (string | undefined)[];
}): PresetGroup[] {
  const reported = catalog.catalog
    ? uniqueTrimmed([catalogValues(catalog.catalog, field, model)])
    : [];
  const fromHarness: PresetGroup = { label: "From harness", options: reported };
  if (!harness) fromHarness.note = "Choose a harness to list its values";
  else if (catalog.loading) fromHarness.note = "Loading…";
  else if (catalog.error) fromHarness.note = catalog.error;
  else if (reported.length === 0) fromHarness.note = "The harness reports no values";

  const saved = runtimePresetOptions(harness, field, extras).filter(
    (value) => !reported.includes(value),
  );
  return [fromHarness, { label: "Saved", options: saved }];
}
