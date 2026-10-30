import {
  type StoreView,
  type StoreImageView,
  type StoreWorkflowView,
  type WorkflowImageListing,
} from "@/lib/api";

/** A catalog row of a Store: one status sentence, its raw detail, and the
 *  one button the row offers (none for an entry that cannot build). */
export interface CatalogRow {
  status: string;
  /** The raw message behind a failure, shown under Details. */
  detail?: string;
  tone: "muted" | "update" | "error";
  action: "build" | "update" | null;
  /** The operation of a failed build of this session. */
  operation?: number;
}

export const catalogRow = (
  image: StoreImageView,
  failure?: { id: number; reason: string },
): CatalogRow => {
  if (image.error)
    return {
      status: "The Store entry is invalid",
      detail: image.error,
      tone: "error",
      action: null,
    };
  if (failure)
    return {
      status: `Build failed: ${failure.reason}`,
      tone: "error",
      action: "build",
      operation: failure.id,
    };
  if (image.latest_status === "error")
    return {
      status: "Could not read the built image",
      detail: image.latest_error || "inspection failed",
      tone: "error",
      action: "build",
    };
  if (image.latest_status === "missing")
    return { status: "Not built", tone: "muted", action: "build" };
  if (image.update_needed)
    return {
      status: `Update available (${image.built_version} → ${image.version})`,
      tone: "update",
      action: "update",
    };
  return { status: "Built", tone: "muted", action: "build" };
};

// latest is the built latest workflow_version: undefined when not built, null
// when the registry could not be read.
export const workflowStatus = (workflow: StoreWorkflowView, latest: string | null | undefined) => {
  if (workflow.error) return `Source error: ${workflow.error}`;
  if (latest === null) return "Comparison unavailable";
  if (latest === undefined) return "Latest not built";
  return latest === workflow.version ? "Up to date" : "Update needed";
};

export const latestWorkflowVersion = (
  workflowImages: WorkflowImageListing[] | null,
  workflowName: string,
) =>
  workflowImages
    ? workflowImages.find((image) => image.name === workflowName && image.tag === "latest")?.version
    : null;

/** Whether a registered Store already is the default Store, by name or by source. */
export const hasTariboyStore = (
  stores: StoreView[],
  defaultStore: { name: string; source: string },
) =>
  stores.some((store) => store.name === defaultStore.name || store.source === defaultStore.source);
