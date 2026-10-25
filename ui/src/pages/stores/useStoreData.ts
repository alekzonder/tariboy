import { useEffect, useRef, useState } from "react";
import type { StoreView, StoreDetail, WorkflowImageListing } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { errorText } from "@/lib/errors";

export function useStoreData(name: string | undefined) {
  const api = useApi();
  const [stores, setStores] = useState<StoreView[]>([]);
  const [detail, setDetail] = useState<StoreDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [autoInterval, setAutoInterval] = useState("0");
  const [autoImages, setAutoImages] = useState<string[]>([]);
  const [workflowImages, setWorkflowImages] = useState<WorkflowImageListing[] | null>(null);
  // The Store whose policy seeded the form: a re-read after a reconnect must
  // not overwrite an unsaved edit.
  const policyLoaded = useRef("");

  useEffect(() => {
    if (!api.ready) return;
    let current = true;
    const showDetail = (loaded: StoreDetail, storeName: string) => {
      setDetail(loaded);
      if (policyLoaded.current !== storeName) {
        policyLoaded.current = storeName;
        setAutoInterval(String(loaded.auto?.interval_minutes ?? 0));
        setAutoImages(loaded.auto?.images ?? []);
      }
    };
    const request = name ? api.stores.getStore(name) : api.stores.listStores();
    void request
      .then((result) => {
        if (!current) return;
        if (name) showDetail(result as StoreDetail, name);
        else setStores(result as StoreView[]);
      })
      .catch((cause) => {
        if (current) setError(errorText(cause));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    if (name) {
      void api.workflows.listWorkflowImages().then(
        (images) => {
          if (current) setWorkflowImages(images);
        },
        () => {
          if (current) setWorkflowImages(null);
        },
      );
    }
    return () => {
      current = false;
    };
  }, [api, name]);

  return {
    stores,
    detail,
    setDetail,
    loading,
    error,
    setError,
    autoInterval,
    setAutoInterval,
    autoImages,
    setAutoImages,
    workflowImages,
    setWorkflowImages,
  };
}

export type StoreData = ReturnType<typeof useStoreData>;
