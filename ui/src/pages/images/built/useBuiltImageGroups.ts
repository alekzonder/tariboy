import { useMemo } from "react";
import type { ImageRow } from "@/lib/api";
import { groupBuiltImages } from "./builtImages";

export function useBuiltImageGroups(images: ImageRow[], imageName?: string) {
  const groups = useMemo(() => groupBuiltImages(images), [images]);
  const tags = groups.find((group) => group.name === imageName)?.tags ?? [];
  return { groups, tags };
}
