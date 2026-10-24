import { useEffect, useState } from "react";
import { ApiError, type ApiClient } from "@/lib/api";
import type { HarnessCatalogState } from "@/lib/runtimePresets";
import type { HarnessCatalog } from "@/lib/types";

// Catalogs without an error are kept for the page session, per host and
// harness; the daemon keeps its own short cache behind this one.
const cache = new Map<string, HarnessCatalog>();

export function clearHarnessCatalogCache(): void {
  cache.clear();
}

const IDLE: HarnessCatalogState = { loading: false, catalog: null, error: "" };
const LOADING: HarnessCatalogState = { loading: true, catalog: null, error: "" };

// `api` is the client of the host whose catalog is wanted: the surrounding
// ApiProvider's, or a dialog's own client for the host it picked.
export function useHarnessCatalog(api: ApiClient, harness: string): HarnessCatalogState {
  const key = harness ? `${api.target?.id ?? ""}\u0000${harness}` : "";
  const [loaded, setLoaded] = useState<{ key: string; state: HarnessCatalogState } | null>(
    null,
  );

  useEffect(() => {
    if (!key || cache.has(key)) return;
    let cancelled = false;
    api.harnesses.getHarnessModels(harness)
      .then((catalog) => {
        if (!catalog.error) cache.set(key, catalog);
        if (!cancelled) {
          setLoaded({ key, state: { loading: false, catalog, error: catalog.error } });
        }
      })
      .catch((cause: unknown) => {
        const error =
          cause instanceof ApiError && cause.status === 404
            ? "This host does not list harness models"
            : cause instanceof Error
              ? cause.message
              : String(cause);
        if (!cancelled) setLoaded({ key, state: { loading: false, catalog: null, error } });
      });
    return () => {
      cancelled = true;
    };
  }, [key, api, harness]);

  if (!key) return IDLE;
  const cached = cache.get(key);
  if (cached) return { loading: false, catalog: cached, error: "" };
  return loaded?.key === key ? loaded.state : LOADING;
}
