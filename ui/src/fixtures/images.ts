import type { ImageSummary } from "@/types";

export const imageSummary = (over: Partial<ImageSummary> = {}): ImageSummary => ({
  name: "worker",
  tag: "v1",
  bare: false,
  built_at: "",
  digest: "",
  exportable: false,
  schema_version: 2,
  current_agents: null,
  pending_agents: null,
  ...over,
});

/** Edge sets of images: `empty`, `one`, `many` (50) and `longText`. */
export const imageSummaries = {
  empty: [] as ImageSummary[],
  one: [imageSummary({ name: "worker", tag: "v1" })],
  many: Array.from({ length: 50 }, (_, i) =>
    imageSummary({ name: `worker-${i + 1}`, tag: `v${i + 1}` }),
  ),
  longText: [
    imageSummary({
      name: "an-image-with-a-very-long-name-that-has-to-truncate-in-the-library-list",
      tag: "latest",
    }),
  ],
};
