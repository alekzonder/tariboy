import { useMemo } from "react";
import type { ImageSummary } from "@/lib/api";
import { groupBuiltImages } from "./builtImages";

export function useBuiltImageGroups(images: ImageSummary[], imageName?: string) {
  const groups = useMemo(() => groupBuiltImages(images), [images]);
  const tags = groups.find((group) => group.name === imageName)?.tags ?? [];
  return { groups, tags };
}
