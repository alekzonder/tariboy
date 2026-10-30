import { useState } from "react";
import { Link } from "react-router-dom";
import StoreCard from "@/pages/stores/StoreCard";
import StoreList from "@/pages/stores/StoreList";
import { useStoreActions } from "@/pages/stores/useStoreActions";
import { useStoreData } from "@/pages/stores/useStoreData";

/** The Library's Sources tab: one card per Store, or the Store `name` alone. */
export default function StoresPage({ name, basePath }: { name?: string; basePath: string }) {
  return (
    <div className="h-full min-h-0 space-y-4 overflow-y-auto p-4">
      {name ? (
        <>
          <Link className="text-sm text-primary hover:underline" to={basePath}>
            ← Sources
          </Link>
          <StoreCard name={name} basePath={basePath} />
        </>
      ) : (
        <SourcesList basePath={basePath} />
      )}
    </div>
  );
}

function SourcesList({ basePath }: { basePath: string }) {
  const [storeName, setStoreName] = useState("");
  const [source, setSource] = useState("");
  const data = useStoreData(undefined);
  const actions = useStoreActions(undefined, basePath, data);
  return (
    <>
      <StoreList
        basePath={basePath}
        stores={data.stores}
        loading={data.loading}
        actions={actions}
        storeName={storeName}
        setStoreName={setStoreName}
        source={source}
        setSource={setSource}
        onRemoved={data.reload}
      />
      {data.error && (
        <p role="alert" className="text-sm text-destructive">
          {data.error}
        </p>
      )}
    </>
  );
}
