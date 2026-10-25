import { useState } from "react";
import StoreDetail from "@/pages/stores/StoreDetail";
import StoreList from "@/pages/stores/StoreList";
import { useStoreActions } from "@/pages/stores/useStoreActions";
import { useStoreData } from "@/pages/stores/useStoreData";

export default function StoresPage({ name, basePath }: { name?: string; basePath: string }) {
  const [storeName, setStoreName] = useState("");
  const [source, setSource] = useState("");
  const data = useStoreData(name);
  const actions = useStoreActions(name, basePath, data);
  const { error } = data;
  const { status } = actions;

  return (
    <div className="h-full min-h-0 space-y-4 overflow-y-auto p-6">
      {name ? (
        <StoreDetail name={name} basePath={basePath} data={data} actions={actions} />
      ) : (
        <StoreList
          basePath={basePath}
          stores={data.stores}
          loading={data.loading}
          actions={actions}
          storeName={storeName}
          setStoreName={setStoreName}
          source={source}
          setSource={setSource}
        />
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {status && (
        <p role="status" className="text-sm text-muted-foreground">
          {status}
        </p>
      )}
    </div>
  );
}
