import type { ApiClient, TeamImportPlan } from "@/lib/api";

type PreviewEntry = TeamImportPlan["agents"][number];

/** A team import preview as the page edits it before confirming. */
export interface ImportPreview {
  import_id: string;
  team: string;
  yaml: string;
  images: Array<{
    original: string;
    resolved: string;
    action: string;
    conflict: boolean;
    message?: string;
  }>;
  agents: PreviewEntry[];
  groups: PreviewEntry[];
  updateExisting: boolean;
  api: ApiClient;
}

/** The editable preview for a YAML or archive import, bound to the host that made it. */
export function previewFromImport(preview: TeamImportPlan, api: ApiClient): ImportPreview {
  return {
    ...preview,
    agents: preview.agents ?? [],
    groups: preview.groups ?? [],
    images: preview.images.map((image) => ({
      ...image,
      original: image.ref,
      resolved: image.ref,
    })),
    updateExisting: false,
    api,
  };
}

/** The image refs the user edited, original to resolved; undefined when none changed. */
export function changedImageRefs(
  images: Array<{ original: string; resolved: string }>,
): Record<string, string> | undefined {
  const refs = Object.fromEntries(
    images
      .filter((image) => image.original !== image.resolved)
      .map((image) => [image.original, image.resolved]),
  );
  return Object.keys(refs).length ? refs : undefined;
}
