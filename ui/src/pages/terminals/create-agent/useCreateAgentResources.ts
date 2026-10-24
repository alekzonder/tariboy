import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { createApi, type ImageManifest, type ImageRow } from "@/lib/api";
import { resolveDaemon, type Daemon } from "@/lib/daemons";
import type { CreateAgentDialogProps } from "../CreateAgentDialog";
import type { AgentCreateDraft } from "../agentCreateDraft";
import { errorText } from "@/lib/errors";
import { draftFromManifest, manifestState } from "./manifestState";

const LOCAL_HOST_VALUE = "__local__";

/** The selected host's daemon and its image list, loaded again whenever the
 *  host or its revision changes. */
function useTargetImages(host: string, selectedHostRevision: string) {
  const [target, setTarget] = useState<Daemon | null>(null);
  const [resolvedRevision, setResolvedRevision] = useState("");
  const [images, setImages] = useState<ImageRow[]>([]);
  const [imagesError, setImagesError] = useState("");

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const nextTarget = host ? await resolveDaemon(host) : null;
        if (host && !nextTarget) throw new Error(`host ${host} was not found`);
        const result = await createApi(nextTarget).images.listImages();
        if (!alive) return;
        setTarget(nextTarget);
        setResolvedRevision(selectedHostRevision);
        setImages(result.images ?? []);
        setImagesError("");
      } catch (error) {
        if (!alive) return;
        setTarget(null);
        setResolvedRevision(selectedHostRevision);
        setImages([]);
        setImagesError(`Could not load images: ${errorText(error)}`);
      }
    })();
    return () => {
      alive = false;
    };
  }, [host, selectedHostRevision]);

  const resetTarget = useCallback(() => {
    setTarget(null);
    setResolvedRevision("");
    setImages([]);
    setImagesError("");
  }, []);

  return { target, resolvedRevision, images, imagesError, resetTarget };
}

/** The selected image's manifest, keyed by host revision and image so a late
 *  answer for an earlier choice is never shown. */
function useImageManifest() {
  const [manifest, setManifest] = useState<ImageManifest | null>(null);
  const [manifestKey, setManifestKey] = useState("");
  const [manifestError, setManifestError] = useState("");
  const [manifestAttempt, setManifestAttempt] = useState(0);

  const loaded = useCallback((key: string, next: ImageManifest | null, error = "") => {
    setManifest(next);
    setManifestKey(key);
    setManifestError(error);
  }, []);
  const resetManifest = () => loaded("", null);
  const retryImage = () => {
    setManifestKey("");
    setManifestError("");
    setManifestAttempt((attempt) => attempt + 1);
  };

  return {
    manifest,
    manifestKey,
    manifestError,
    manifestAttempt,
    loaded,
    resetManifest,
    retryImage,
  };
}

export function useCreateAgentResources(
  { hostId, hosts, cloneSource }: CreateAgentDialogProps,
  draft: AgentCreateDraft,
  setDraft: Dispatch<SetStateAction<AgentCreateDraft>>,
  updateDraft: <K extends keyof AgentCreateDraft>(key: K, value: AgentCreateDraft[K]) => void,
) {
  const [host, setHost] = useState(hostId);
  const selectedHostRevision = hosts.find((entry) => entry.id === host)?.revision ?? "missing";
  const { target, resolvedRevision, images, imagesError, resetTarget } = useTargetImages(
    host,
    selectedHostRevision,
  );
  const {
    manifest,
    manifestKey,
    manifestError,
    manifestAttempt,
    loaded,
    resetManifest,
    retryImage,
  } = useImageManifest();
  // The dialog picks its own host, so it holds its own client: every
  // follow-up request stays pinned to the host the images were listed from.
  const api = useMemo(() => createApi(target), [target]);
  const runtimeInitializedForTarget = useRef("");
  const targetIsCurrent = resolvedRevision === selectedHostRevision;
  const targetIsUsable = host === "" || target !== null;
  const loadingImages = !targetIsCurrent;
  const expectedManifestKey = `${selectedHostRevision}\u0000${draft.image}`;
  const runtimeTargetKey = `${host}\u0000${draft.image}`;
  const state = manifestState({
    image: draft.image,
    images,
    manifest,
    manifestKey,
    expectedManifestKey,
    targetIsCurrent,
    targetIsUsable,
    draftPlugins: draft.plugins,
  });
  const { targetImageAvailable } = state;

  useEffect(() => {
    if (!draft.image || loadingImages || !targetIsCurrent || !targetIsUsable) return;
    if (!targetImageAvailable) return;
    let alive = true;
    void api.images
      .imageManifestGet(draft.image)
      .then((next) => {
        if (!alive) return;
        loaded(expectedManifestKey, next);
        if (!cloneSource && runtimeInitializedForTarget.current !== runtimeTargetKey) {
          runtimeInitializedForTarget.current = runtimeTargetKey;
          setDraft((current) => draftFromManifest(current, next));
        }
      })
      .catch((error) => {
        if (!alive) return;
        loaded(
          expectedManifestKey,
          null,
          `Could not load image ${draft.image}: ${errorText(error)}`,
        );
      });
    return () => {
      alive = false;
    };
  }, [
    cloneSource,
    draft.image,
    expectedManifestKey,
    loadingImages,
    manifestAttempt,
    runtimeTargetKey,
    api,
    targetImageAvailable,
    targetIsCurrent,
    targetIsUsable,
    setDraft,
    loaded,
  ]);

  const changeHost = (value: string) => {
    const next = value === LOCAL_HOST_VALUE ? "" : value;
    if (next === host) return;
    setHost(next);
    resetTarget();
    resetManifest();
    if (!cloneSource) updateDraft("image", "");
  };

  const changeImage = (image: string) => {
    updateDraft("image", image);
    resetManifest();
  };

  return {
    ...state,
    host,
    target,
    api,
    images,
    loadingImages,
    targetIsCurrent,
    targetIsUsable,
    expectedManifestKey,
    manifestKey,
    manifestError,
    imagesError,
    changeHost,
    changeImage,
    retryImage,
  };
}
