import { useCallback, useEffect, useRef, useState } from "react";
import {
  ApiError,
  createApi,
  type ApiClient,
  type ImageManifest,
  type ImageProvenance,
} from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import { errorText } from "@/lib/errors";

export interface ImageDetailData {
  manifest: ImageManifest;
  provenance: ImageProvenance;
  api: ApiClient;
}

export function useImageDetail(hostKey: string, ref: string) {
  const [detail, setDetail] = useState<ImageDetailData | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [revision, setRevision] = useState(0);
  const refreshes = useRef(0);
  const onReadError = useCallback((cause: unknown) => {
    setDetail(null);
    if (cause instanceof ApiError && cause.code === "image_changed") {
      setNotice("Image changed; refreshing the displayed build.");
      if (refreshes.current++ === 0) {
        setRevision((value) => value + 1);
        return;
      }
    }
    setError(errorText(cause));
  }, []);

  useEffect(() => {
    let alive = true;
    void (async () => {
      const target = await resolveDaemon(hostKey);
      if (hostKey && !target) throw new Error(`host ${hostKey} is not available`);
      // The image's host is resolved here, so this layout is the server
      // boundary of its tabs: they read through the same client.
      const api = createApi(target);
      const manifest = await api.images.imageManifestGet(ref);
      const provenance = await api.images.imageProvenanceGet(ref, manifest.digest);
      if (provenance.digest && provenance.digest !== manifest.digest)
        throw new ApiError(409, "image_changed", "Image changed; refresh to inspect the new build");
      if (alive) {
        setDetail({ manifest, provenance, api });
        setError("");
      }
    })().catch((cause) => {
      if (alive) onReadError(cause);
    });
    return () => {
      alive = false;
    };
  }, [hostKey, ref, revision, onReadError]);

  const refresh = () => {
    refreshes.current = 0;
    setError("");
    setRevision((value) => value + 1);
  };

  return { detail, error, notice, onReadError, refresh };
}
