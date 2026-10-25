import {
  type StoreView,
  type StoreImageView,
  type StoreWorkflowView,
  TARIBOY_STORE,
  type WorkflowImage,
} from "@/lib/api";

const builtComparison = (image: StoreImageView) => {
  if (!image.version) return "Source image_version missing; comparison unavailable";
  return image.update_needed ? "Update needed" : "Up to date";
};

const comparisonStatus = (image: StoreImageView) => {
  if (
    image.latest_status === "unversioned" ||
    (image.latest_status === "built" && !image.built_version)
  ) {
    return "Latest image_version missing; comparison unavailable";
  }
  if (image.latest_status === "built" && !image.error) return builtComparison(image);
  if (!image.latest_status && !image.error) return "Comparison unavailable";
  return "";
};

export const imageStatuses = (image: StoreImageView) => {
  const statuses: string[] = [];
  if (image.error) statuses.push(`Source error: ${image.error}`);
  if (image.latest_status === "error")
    statuses.push(`Latest error: ${image.latest_error || "inspection failed"}`);
  if (image.latest_status === "missing") statuses.push("Latest not built");
  const comparison = comparisonStatus(image);
  if (comparison) statuses.push(comparison);
  return statuses;
};

export const latestImageVersionLabel = (image: StoreImageView) => {
  if (image.latest_status === "missing") return "Not built";
  if (image.latest_status === "error") return "Inspection failed";
  return image.built_version || "Not specified";
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
  workflowImages: WorkflowImage[] | null,
  workflowName: string,
) =>
  workflowImages
    ? workflowImages.find((image) => image.name === workflowName && image.tag === "latest")?.version
    : null;

export const hasTariboyStore = (stores: StoreView[]) =>
  stores.some(
    (store) => store.name === TARIBOY_STORE.name || store.source === TARIBOY_STORE.source,
  );
