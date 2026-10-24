import type { ImageManifest, ImageRow } from "@/lib/api";
import type { AgentCreateDraft } from "../agentCreateDraft";

export interface ManifestInputs {
  image: string;
  images: ImageRow[];
  manifest: ImageManifest | null;
  manifestKey: string;
  expectedManifestKey: string;
  targetIsCurrent: boolean;
  targetIsUsable: boolean;
  draftPlugins: string[];
}

/** What the loaded manifest offers the form, once it matches the selected image. */
function manifestDefaults(current: ImageManifest | null, draftPlugins: string[]) {
  const schemaV1 = current?.schema_version === 1;
  const schemaV2 = current?.schema_version === 2;
  return {
    bare: current?.bare === true,
    schemaV2,
    defaultModel: schemaV1 ? (current.harness?.model ?? "") : "",
    defaultEffort: schemaV1 ? (current.harness?.effort ?? "") : "",
    displayedPlugins: schemaV2
      ? (current.plugins?.map((plugin) => plugin.name) ?? [])
      : draftPlugins,
  };
}

/** Whether the selected image's manifest is loaded, loading, and what it sets. */
export function manifestState(inputs: ManifestInputs) {
  const { image, manifest, manifestKey, expectedManifestKey } = inputs;
  const targetImageAvailable = inputs.images.some(
    (entry) => `${entry.name}:${entry.tag}` === image,
  );
  const selectable = image !== "" && targetImageAvailable;
  const manifestIsCurrent = selectable && manifestKey === expectedManifestKey && manifest !== null;
  const loadingManifest =
    selectable &&
    inputs.targetIsCurrent &&
    inputs.targetIsUsable &&
    manifestKey !== expectedManifestKey;
  return {
    targetImageAvailable,
    manifestIsCurrent,
    loadingManifest,
    ...manifestDefaults(manifestIsCurrent ? manifest : null, inputs.draftPlugins),
  };
}

/** A new agent takes its harness, model and effort from a schema 1 image. */
export function draftFromManifest(
  current: AgentCreateDraft,
  next: ImageManifest,
): AgentCreateDraft {
  if (next.schema_version !== 1) return current;
  return {
    ...current,
    harness: next.harness?.type ?? "",
    model: next.harness?.model ?? "",
    effort: next.harness?.effort ?? "",
  };
}
