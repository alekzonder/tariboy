import { type MutableRefObject, useEffect, useRef, useState } from "react";
import { type NavigateFunction, useNavigate } from "react-router-dom";
import type { ApiClient, OperationStarted } from "@/lib/api";
import { notifyImagesChanged } from "@/lib/imageEvents";
import { useApi } from "@/components/apiContext";
import { useOperationHref, useOperationStartedToast } from "@/hooks/useOperationStartedToast";
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
  buildStarted: (started: OperationStarted) => void;
  builds: BuildFailures;
}

/** Why the last build of an image failed, shown in its row. */
export interface ImageBuildError {
  id: number;
  reason: string;
}

interface TrackedBuild {
  api: ApiClient;
  store: string;
  image: string;
}

// BuildFailures remembers which image each started build is for, so a failure
// reported by operation id lands in that image's row. An entry is tied to the
// client and the Store it was started on: another host or Store shows none.
function useBuildFailures(api: ApiClient, name: string | undefined) {
  const tracked = useRef(new Map<number, TrackedBuild>());
  const [failures, setFailures] = useState<(TrackedBuild & ImageBuildError)[]>([]);
  const clear = (store: string, image: string) =>
    setFailures((current) =>
      current.filter((failure) => failure.store !== store || failure.image !== image),
    );
  const track = (id: number, store: string, image: string) => {
    tracked.current.set(id, { api, store, image });
    clear(store, image);
  };
  const failed = (id: number) => {
    const build = tracked.current.get(id);
    if (!build) return;
    tracked.current.delete(id);
    void build.api.operations
      .get(id)
      .then(
        (operation) => operation.error_message || `Build ${operation.status}`,
        () => "Read the operation log for the reason",
      )
      .then((reason) => setFailures((current) => [...current, { ...build, id, reason }]));
  };
  const errors: Record<string, ImageBuildError> = {};
  for (const failure of failures) {
    if (failure.api === api && failure.store === name) errors[failure.image] = failure;
  }
  return { track, clear, failed, errors };
}

type BuildFailures = ReturnType<typeof useBuildFailures>;

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

// A build only starts here: the daemon runs it as an operation, and the
// toast links to its page. The store reloads when the build succeeds.
const startBuild = async (
  ctx: StoreActionContext,
  busyKey: string,
  request: () => Promise<OperationStarted>,
  onStarted?: (id: number) => void,
) => {
  const isCurrent = currentBuild(ctx);
  start(ctx, busyKey);
  try {
    const started = await request();
    // A build on a host the page has since left is not reported against this one.
    if (isCurrent()) {
      onStarted?.(started.operation.id);
      ctx.buildStarted(started);
      ctx.setStatus(`Build started: ${started.operation.title}.`);
    }
  } catch (cause) {
    if (isCurrent()) ctx.data.setError(errorText(cause));
  } finally {
    if (isCurrent()) ctx.setBusy("");
  }
};

// A new build of an image clears the failure its row shows.
const buildImage = (ctx: StoreActionContext, name: string, imageName: string) => {
  ctx.builds.clear(name, imageName);
  return startBuild(
    ctx,
    `build:${imageName}`,
    () => ctx.api.stores.buildStoreImage({ source: `${name}/${imageName}` }),
    (id) => ctx.builds.track(id, name, imageName),
  );
};

const buildWorkflowImage = (ctx: StoreActionContext, name: string, workflowName: string) =>
  startBuild(ctx, `workflow:${workflowName}`, () =>
    ctx.api.stores.buildStoreWorkflow(`${name}/${workflowName}`),
  );

// reloadAfterBuild re-reads what a finished build changes: the store's image
// states and the built workflow images. A Refresh or another build started
// meanwhile owns the detail, and this late read is dropped.
const reloadAfterBuild = async (ctx: StoreActionContext, name: string) => {
  notifyImagesChanged();
  const generation = ctx.buildGeneration.current;
  const isCurrent = () => ctx.mounted.current && ctx.buildGeneration.current === generation;
  try {
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
  const builtRef = useRef<() => void>(() => undefined);
  const builds = useBuildFailures(api, name);
  const failedRef = useRef(builds.failed);
  const buildStarted = useOperationStartedToast(
    () => builtRef.current(),
    (id) => failedRef.current(id),
  );
  const operationHref = useOperationHref();

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
    buildStarted,
    builds,
  };
  useEffect(() => {
    builtRef.current = () => {
      if (name) void reloadAfterBuild(ctx, name);
    };
    failedRef.current = builds.failed;
  });
  return {
    busy,
    status,
    buildErrors: builds.errors,
    operationHref,
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
