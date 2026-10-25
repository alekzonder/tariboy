import { useCallback, useEffect, useRef, useState } from "react";
import {
  isConnectionError,
  type AgentImageStatus,
  type ImageSummary,
  type AgentStatus,
  type AgentView,
  type RetentionView,
  type SecretKeyList,
} from "@/lib/api";
import { subscribeImagesChanged } from "@/lib/imageEvents";
import { useApi } from "@/components/apiContext";
import { errorText } from "@/lib/errors";

// A loaded value, or the reason it is missing. A failed reload keeps the last
// value: a failed read is not an empty one.
interface Loaded<T> {
  value: T | null;
  error: string;
}

export interface ConfigSources {
  view: AgentView | null;
  imageStatus: AgentImageStatus | null;
  imageError: string;
  images: ImageSummary[];
  inventoryLoading: boolean;
  inventoryError: string;
  script: Loaded<string>;
  retention: Loaded<RetentionView>;
  secrets: Loaded<string[]>;
  loadImages: () => Promise<void>;
  reloadAll: () => Promise<void>;
}

const empty = <T>(): Loaded<T> => ({ value: null, error: "" });

// useConfigSources is the page's one load point. A scope is one agent on one
// host: a reconnect moves the endpoint but keeps the scope, so loaded values
// survive it, while a response that arrives after the scope changed is
// dropped. The page keys its draft by the same scope. The host is the
// surrounding ApiProvider's.
export function useConfigSources(name: string, status: AgentStatus | null): ConfigSources {
  const api = useApi();
  const [view, setView] = useState<AgentView | null>(null);
  const [imageStatus, setImageStatus] = useState<AgentImageStatus | null>(null);
  const [imageError, setImageError] = useState("");
  const [images, setImages] = useState<ImageSummary[]>([]);
  const [inventoryLoading, setInventoryLoading] = useState(true);
  const [inventoryError, setInventoryError] = useState("");
  const [script, setScript] = useState<Loaded<string>>(empty);
  const [retention, setRetention] = useState<Loaded<RetentionView>>(empty);
  const [secrets, setSecrets] = useState<Loaded<string[]>>(empty);
  const targetId = api.target?.id;

  const requests = useRef({ name, targetId, active: false, agent: 0, image: 0 });
  useEffect(() => {
    const scope = { name, targetId, active: true, agent: 0, image: 0 };
    requests.current = scope;
    void Promise.resolve().then(() => {
      if (!scope.active) return;
      setView(null);
      setImageStatus(null);
      setImageError("");
      setImages([]);
      setInventoryError("");
      setInventoryLoading(true);
      setScript(empty());
      setRetention(empty());
      setSecrets(empty());
    });
    return () => {
      scope.active = false;
    };
  }, [name, targetId]);

  const loadAgent = useCallback(async () => {
    const scope = requests.current;
    if (!scope.active || scope.name !== name || scope.targetId !== targetId) return;
    if (!api.ready) return;
    const request = ++scope.agent;
    const current = () => scope.active && request === scope.agent;
    const keep =
      <T>(set: (update: (prev: Loaded<T>) => Loaded<T>) => void) =>
      (cause: unknown) => {
        if (current())
          set((prev) => ({
            value: prev.value,
            error: prev.value === null ? errorText(cause) : "",
          }));
      };
    await Promise.all([
      api.agents.getAgent(name).then(
        (next) => {
          if (current()) setView(next);
        },
        () => {
          // Keep the last loaded agent: a failed read is not an empty agent.
        },
      ),
      api.agents.getAgentShellScript(name).then((next) => {
        if (current())
          setScript({ value: typeof next?.script === "string" ? next.script : "", error: "" });
      }, keep(setScript)),
      api.agentGet<RetentionView>(name, "retention").then((next) => {
        if (current()) setRetention({ value: next, error: "" });
      }, keep(setRetention)),
      api.agentGet<SecretKeyList>(name, "secrets").then((next) => {
        if (current()) setSecrets({ value: Array.isArray(next?.keys) ? next.keys : [], error: "" });
      }, keep(setSecrets)),
    ]);
  }, [name, api, targetId]);

  useEffect(() => {
    void Promise.resolve().then(loadAgent);
  }, [loadAgent]);

  const loadImages = useCallback(async () => {
    const scope = requests.current;
    if (!scope.active || scope.name !== name || scope.targetId !== targetId) return;
    if (!api.ready) return;
    const request = ++scope.image;
    setInventoryLoading(true);
    void api.images
      .listImages()
      .then((listing) => {
        if (!scope.active || request !== scope.image) return;
        setImages(listing.images ?? []);
        setInventoryError("");
      })
      .catch((cause) => {
        if (!scope.active || request !== scope.image) return;
        // An unreachable host keeps the last inventory; the error still shows.
        if (!isConnectionError(cause)) setImages([]);
        setInventoryError(errorText(cause));
      })
      .finally(() => {
        if (scope.active && request === scope.image) setInventoryLoading(false);
      });
    try {
      const state = await api.agents.agentImageStatusGet(name);
      if (!scope.active || request !== scope.image) return;
      setImageStatus(state);
      setImageError("");
    } catch (cause) {
      if (!scope.active || request !== scope.image) return;
      if (!isConnectionError(cause)) setImageStatus(null);
      setImageError(errorText(cause));
    }
  }, [name, api, targetId]);

  useEffect(() => {
    const scope = requests.current;
    void Promise.resolve().then(loadImages);
    return () => {
      scope.image++;
    };
  }, [loadImages, status]);
  useEffect(
    () =>
      subscribeImagesChanged(() => {
        void loadImages();
      }),
    [loadImages],
  );

  const reloadAll = useCallback(async () => {
    await Promise.all([loadAgent(), loadImages()]);
  }, [loadAgent, loadImages]);

  return {
    view,
    imageStatus,
    imageError,
    images,
    inventoryLoading,
    inventoryError,
    script,
    retention,
    secrets,
    loadImages,
    reloadAll,
  };
}
