import { type MutableRefObject, useEffect, useRef, useState } from "react";
import { type NavigateFunction, useNavigate } from "react-router-dom";
import type { ApiClient } from "@/lib/api";
import { notifyImagesChanged } from "@/lib/imageEvents";
import { useApi } from "@/components/apiContext";
import { errorText } from "@/lib/errors";
import type { StoreData } from "./useStoreData";

interface StoreActionContext {
  api: ApiClient;
  navigate: NavigateFunction;
  basePath: string;
  data: StoreData;
  mounted: MutableRefObject<boolean>;
  buildGeneration: MutableRefObject<number>;
  setBusy: (busy: string) => void;
  setStatus: (status: string) => void;
  setRemoveError: (error: string) => void;
}

const start = (ctx: StoreActionContext, busyKey: string) => {
  ctx.setBusy(busyKey);
  ctx.data.setError("");
  ctx.setStatus("");
};

const addStore = async (
  ctx: StoreActionContext,
  input: { name: string; source: string },
  busyKey: string,
) => {
  if (!input.name || !input.source) return;
  start(ctx, busyKey);
  try {
    const added = await ctx.api.stores.addStore(input);
    if (ctx.mounted.current) ctx.navigate(`${ctx.basePath}/${encodeURIComponent(added.name)}`);
  } catch (cause) {
    if (ctx.mounted.current) ctx.data.setError(errorText(cause));
  } finally {
    if (ctx.mounted.current) ctx.setBusy("");
  }
};

const refreshStore = async (ctx: StoreActionContext, name: string) => {
  ctx.buildGeneration.current++;
  start(ctx, "refresh");
  try {
    const refreshed = await ctx.api.stores.refreshStore(name);
    if (ctx.mounted.current) {
      ctx.data.setDetail(refreshed);
      ctx.setStatus(`Refreshed ${name}.`);
    }
  } catch (cause) {
    if (ctx.mounted.current) ctx.data.setError(errorText(cause));
  } finally {
    if (ctx.mounted.current) ctx.setBusy("");
  }
};

const currentBuild = (ctx: StoreActionContext) => {
  const requestedBuild = ++ctx.buildGeneration.current;
  return () => ctx.mounted.current && ctx.buildGeneration.current === requestedBuild;
};

const buildImage = async (ctx: StoreActionContext, name: string, imageName: string) => {
  const isCurrent = currentBuild(ctx);
  start(ctx, `build:${imageName}`);
  try {
    const built = await ctx.api.stores.buildStoreImage({ source: `${name}/${imageName}` });
    if (isCurrent()) {
      ctx.setStatus(
        built.tag === "latest"
          ? `Built ${built.name}:latest.`
          : `Built ${built.name}:${built.tag} and ${built.name}:latest.`,
      );
      ctx.setBusy("");
      notifyImagesChanged();
    }
    if (!isCurrent()) return;
    try {
      const refreshed = await ctx.api.stores.getStore(name);
      if (isCurrent()) ctx.data.setDetail(refreshed);
    } catch (cause) {
      if (isCurrent()) ctx.data.setError(errorText(cause));
    }
  } catch (cause) {
    if (isCurrent()) ctx.data.setError(errorText(cause));
  } finally {
    if (isCurrent()) ctx.setBusy("");
  }
};

const buildWorkflowImage = async (ctx: StoreActionContext, name: string, workflowName: string) => {
  const isCurrent = currentBuild(ctx);
  start(ctx, `workflow:${workflowName}`);
  try {
    const built = await ctx.api.stores.buildStoreWorkflow(`${name}/${workflowName}`);
    if (!isCurrent()) return;
    ctx.setStatus(`Built ${built.name} ${built.version} (tags: ${built.tags.join(", ")}).`);
    ctx.setBusy("");
    const [refreshed, images] = await Promise.all([
      ctx.api.stores.getStore(name),
      ctx.api.workflows.listWorkflowImages().catch(() => null),
    ]);
    if (isCurrent()) {
      ctx.data.setDetail(refreshed);
      ctx.data.setWorkflowImages(images);
    }
  } catch (cause) {
    if (isCurrent()) ctx.data.setError(errorText(cause));
  } finally {
    if (isCurrent()) ctx.setBusy("");
  }
};

const saveStoreAuto = async (ctx: StoreActionContext, name: string) => {
  const { data } = ctx;
  const interval = Number(data.autoInterval);
  if (!Number.isInteger(interval) || interval < 0) {
    data.setError("Automatic build interval must be a whole number of minutes.");
    return;
  }
  start(ctx, "auto");
  try {
    const saved = await ctx.api.stores.setStoreAuto(name, { interval, image: data.autoImages });
    if (ctx.mounted.current) {
      data.setDetail(saved);
      data.setAutoInterval(String(saved.auto?.interval_minutes ?? 0));
      data.setAutoImages(saved.auto?.images ?? []);
      ctx.setStatus(
        interval > 0
          ? `Automatic builds run every ${interval} min for ${data.autoImages.length} image(s).`
          : "Automatic builds disabled.",
      );
    }
  } catch (cause) {
    if (ctx.mounted.current) data.setError(errorText(cause));
  } finally {
    if (ctx.mounted.current) ctx.setBusy("");
  }
};

const removeStore = async (ctx: StoreActionContext, name: string) => {
  ctx.buildGeneration.current++;
  ctx.setBusy("remove");
  ctx.data.setError("");
  ctx.setRemoveError("");
  ctx.setStatus("");
  try {
    await ctx.api.stores.removeStore(name);
    if (ctx.mounted.current) ctx.navigate(ctx.basePath);
  } catch (cause) {
    if (ctx.mounted.current) {
      const failure = errorText(cause);
      ctx.data.setError(failure);
      ctx.setRemoveError(failure);
    }
  } finally {
    if (ctx.mounted.current) ctx.setBusy("");
  }
};

export function useStoreActions(name: string | undefined, basePath: string, data: StoreData) {
  const api = useApi();
  const navigate = useNavigate();
  const mounted = useRef(true);
  const buildGeneration = useRef(0);
  const [busy, setBusy] = useState("");
  const [status, setStatus] = useState("");
  const [removeError, setRemoveError] = useState("");
  const [removeOpen, setRemoveOpen] = useState(false);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const ctx: StoreActionContext = {
    api,
    navigate,
    basePath,
    data,
    mounted,
    buildGeneration,
    setBusy,
    setStatus,
    setRemoveError,
  };
  return {
    busy,
    status,
    removeError,
    removeOpen,
    changeRemoveOpen: (open: boolean) => {
      if (busy === "remove") return;
      if (open) setRemoveError("");
      setRemoveOpen(open);
    },
    add: (input: { name: string; source: string }, busyKey = "add") =>
      addStore(ctx, input, busyKey),
    refresh: async () => {
      if (name) await refreshStore(ctx, name);
    },
    build: async (imageName: string) => {
      if (name) await buildImage(ctx, name, imageName);
    },
    buildWorkflow: async (workflowName: string) => {
      if (name) await buildWorkflowImage(ctx, name, workflowName);
    },
    saveAuto: async () => {
      if (name) await saveStoreAuto(ctx, name);
    },
    remove: async () => {
      if (name) await removeStore(ctx, name);
    },
  };
}

export type StoreActions = ReturnType<typeof useStoreActions>;
