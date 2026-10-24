import { useOutletContext } from "react-router-dom";
import type { ImageManifest, ImageProvenance } from "@/lib/api";

export interface ImageOutletContext {
  ref: string;
  manifest: ImageManifest | null;
  hostKey: string;
  provenance: ImageProvenance | null;
  onReadError: (error: unknown) => void;
}

export function useImageContext() {
  return useOutletContext<ImageOutletContext>();
}
