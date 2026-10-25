import { Outlet, useLocation } from "react-router-dom";
import { ApiProvider } from "@/components/ApiProvider";
import { LazyBoundary } from "@/components/LazyBoundary";
import { Button } from "@/components/ui/button";
import type { ImageOutletContext } from "../imageContext";
import { imageDetailBodyState } from "./imageDetailState";
import type { ImageDetailData } from "./useImageDetail";

export function ImageDetailBody({
  imageRef,
  hostKey,
  detail,
  error,
  onReadError,
  onRefresh,
}: {
  imageRef: string;
  hostKey: string;
  detail: ImageDetailData | null;
  error: string;
  onReadError: (error: unknown) => void;
  onRefresh: () => void;
}) {
  const location = useLocation();
  const state = imageDetailBodyState(error, !!detail);
  return (
    <div className="flex-1 overflow-auto p-4">
      {state === "error" && (
        <div className="space-y-2">
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
          <Button variant="outline" onClick={onRefresh}>
            Refresh image
          </Button>
        </div>
      )}
      {state === "loaded" && detail && (
        <ApiProvider client={detail.api}>
          <LazyBoundary resetKey={location.pathname}>
            <Outlet
              key={detail.manifest.digest}
              context={
                {
                  ref: imageRef,
                  manifest: detail.manifest,
                  hostKey,
                  provenance: detail.provenance,
                  onReadError,
                } satisfies ImageOutletContext
              }
            />
          </LazyBoundary>
        </ApiProvider>
      )}
      {state === "loading" && <p className="text-sm text-muted-foreground">Loading…</p>}
    </div>
  );
}
