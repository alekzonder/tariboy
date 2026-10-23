import { useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { imageManifestGet, listImages, type ImageManifest, type ImageRow } from "@/lib/api";
import { resolveDaemon, type Daemon } from "@/lib/daemons";
import type { CreateAgentDialogProps } from "../CreateAgentDialog";
import type { AgentCreateDraft } from "../agentCreateDraft";
import { errorMessage } from "./errorMessage";

const LOCAL_HOST_VALUE = "__local__";

export function useCreateAgentResources(
  { hostId, hosts, cloneSource }: CreateAgentDialogProps,
  draft: AgentCreateDraft,
  setDraft: Dispatch<SetStateAction<AgentCreateDraft>>,
  updateDraft: <K extends keyof AgentCreateDraft>(key: K, value: AgentCreateDraft[K]) => void,
) {
  const [host, setHost] = useState(hostId);
  const [target, setTarget] = useState<Daemon | null>(null);
  const [resolvedRevision, setResolvedRevision] = useState("");
  const [images, setImages] = useState<ImageRow[]>([]);
  const [manifest, setManifest] = useState<ImageManifest | null>(null);
  const [manifestKey, setManifestKey] = useState("");
  const [manifestError, setManifestError] = useState("");
  const [manifestAttempt, setManifestAttempt] = useState(0);
  const runtimeInitializedForTarget = useRef("");
  const [imagesError, setImagesError] = useState("");
  const selectedHostRevision =
    hosts.find((entry) => entry.id === host)?.revision ?? "missing";
  const targetIsCurrent = resolvedRevision === selectedHostRevision;
  const targetIsUsable = host === "" || target !== null;
  const loadingImages = !targetIsCurrent;
  const targetImageAvailable = images.some(
    (entry) => `${entry.name}:${entry.tag}` === draft.image,
  );
  const expectedManifestKey = `${selectedHostRevision}\u0000${draft.image}`;
  const runtimeTargetKey = `${host}\u0000${draft.image}`;
  const manifestIsCurrent =
    draft.image !== "" &&
    targetImageAvailable &&
    manifestKey === expectedManifestKey &&
    manifest !== null;
  const loadingManifest =
    draft.image !== "" &&
    targetIsCurrent &&
    targetIsUsable &&
    targetImageAvailable &&
    manifestKey !== expectedManifestKey;
  const currentManifest = manifestIsCurrent ? manifest : null;
  const bare = currentManifest?.bare === true;
  const schemaV2 = currentManifest?.schema_version === 2;
  const defaultModel =
    currentManifest?.schema_version === 1
      ? (currentManifest.harness?.model ?? "")
      : "";
  const defaultEffort =
    currentManifest?.schema_version === 1
      ? (currentManifest.harness?.effort ?? "")
      : "";
  const displayedPlugins = schemaV2
    ? (currentManifest?.plugins?.map((plugin) => plugin.name) ?? [])
    : draft.plugins;
  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const nextTarget = host ? await resolveDaemon(host) : null;
        if (host && !nextTarget) throw new Error(`host ${host} was not found`);
        const result = await listImages(nextTarget);
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
        setImagesError(`Could not load images: ${errorMessage(error)}`);
      }
    })();
    return () => {
      alive = false;
    };
  }, [host, selectedHostRevision]);

  useEffect(() => {
    if (!draft.image || loadingImages || !targetIsCurrent || !targetIsUsable)
      return;
    if (!targetImageAvailable) return;
    let alive = true;
    void imageManifestGet(draft.image, target)
      .then((next) => {
        if (!alive) return;
        setManifest(next);
        setManifestKey(expectedManifestKey);
        setManifestError("");
        if (
          !cloneSource &&
          runtimeInitializedForTarget.current !== runtimeTargetKey
        ) {
          runtimeInitializedForTarget.current = runtimeTargetKey;
          setDraft((current) => ({
            ...current,
            harness:
              next.schema_version === 1
                ? (next.harness?.type ?? "")
                : current.harness,
            model:
              next.schema_version === 1
                ? (next.harness?.model ?? "")
                : current.model,
            effort:
              next.schema_version === 1
                ? (next.harness?.effort ?? "")
                : current.effort,
          }));
        }
      })
      .catch((error) => {
        if (!alive) return;
        setManifest(null);
        setManifestKey(expectedManifestKey);
        setManifestError(
          `Could not load image ${draft.image}: ${errorMessage(error)}`,
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
    target,
    targetImageAvailable,
    targetIsCurrent,
    targetIsUsable,
    setDraft,
  ]);

  const changeHost = (value: string) => {
    const next = value === LOCAL_HOST_VALUE ? "" : value;
    if (next === host) return;
    setHost(next);
    setTarget(null);
    setResolvedRevision("");
    setImages([]);
    setManifest(null);
    setManifestKey("");
    setManifestError("");
    setImagesError("");
    if (!cloneSource) updateDraft("image", "");
  };

  const changeImage = (image: string) => {
    updateDraft("image", image);
    setManifest(null);
    setManifestKey("");
    setManifestError("");
  };

  const retryImage = () => {
    setManifestKey("");
    setManifestError("");
    setManifestAttempt((attempt) => attempt + 1);
  };

  return {
    host,
    target,
    images,
    loadingImages,
    loadingManifest,
    targetIsCurrent,
    targetIsUsable,
    targetImageAvailable,
    expectedManifestKey,
    manifestKey,
    manifestError,
    imagesError,
    manifestIsCurrent,
    bare,
    schemaV2,
    defaultModel,
    defaultEffort,
    displayedPlugins,
    changeHost,
    changeImage,
    retryImage,
  };
}
