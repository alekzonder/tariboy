import { Link } from "react-router-dom";
import { type Store, TARIBOY_STORE } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { StoreActions } from "./useStoreActions";
import { hasTariboyStore } from "./storeStatus";

interface StoreListProps {
  basePath: string;
  stores: Store[];
  loading: boolean;
  actions: StoreActions;
  storeName: string;
  setStoreName: (name: string) => void;
  source: string;
  setSource: (source: string) => void;
}

function TariboyStoreCard({ actions }: Pick<StoreListProps, "actions">) {
  return (
    <section className="flex flex-wrap items-center justify-between gap-3 rounded border p-4">
      <div className="min-w-0 space-y-1">
        <h2 className="font-medium">Tariboy Store</h2>
        <p className="break-all text-sm text-muted-foreground">
          Images and workflows published by Tariboy, registered as{" "}
          <span className="font-mono">{TARIBOY_STORE.name}</span> from{" "}
          <span className="font-mono">{TARIBOY_STORE.source}</span>.
        </p>
      </div>
      <Button
        disabled={Boolean(actions.busy)}
        onClick={() => void actions.add(TARIBOY_STORE, "add-tariboy")}
      >
        {actions.busy === "add-tariboy" ? "Adding…" : "Add tariboy Store"}
      </Button>
    </section>
  );
}

function AddStoreForm({ actions, storeName, setStoreName, source, setSource }: StoreListProps) {
  const { busy } = actions;
  return (
    <section className="space-y-3 rounded border p-4">
      <h2 className="font-medium">Add Store</h2>
      <div className="grid gap-3 md:grid-cols-[14rem_minmax(0,1fr)_auto]">
        <Input
          aria-label="Store name"
          required
          placeholder="name"
          value={storeName}
          disabled={Boolean(busy)}
          onChange={(event) => setStoreName(event.target.value)}
        />
        <Input
          aria-label="Store source"
          required
          placeholder="Git URL or absolute directory"
          value={source}
          disabled={Boolean(busy)}
          onChange={(event) => setSource(event.target.value)}
        />
        <Button
          disabled={Boolean(busy) || !storeName.trim() || !source.trim()}
          onClick={() => void actions.add({ name: storeName.trim(), source: source.trim() })}
        >
          {busy === "add" ? "Adding…" : "Add store"}
        </Button>
      </div>
    </section>
  );
}

function StoresTable({ basePath, stores }: Pick<StoreListProps, "basePath" | "stores">) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2">Store</th>
            <th className="px-3 py-2">Source</th>
          </tr>
        </thead>
        <tbody>
          {stores.map((store) => (
            <tr key={store.name} className="border-t">
              <td className="px-3 py-2">
                <Link
                  className="font-medium text-primary hover:underline"
                  to={`${basePath}/${encodeURIComponent(store.name)}`}
                >
                  {store.name}
                </Link>
              </td>
              <td className="break-all px-3 py-2 font-mono text-xs text-muted-foreground">
                {store.source}
              </td>
            </tr>
          ))}
          {stores.length === 0 && (
            <tr>
              <td colSpan={2} className="px-3 py-8 text-center text-muted-foreground">
                No Stores registered on this server.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

export default function StoreList(props: StoreListProps) {
  const { loading, stores } = props;
  return (
    <>
      <div>
        <h1 className="text-lg font-semibold">Stores</h1>
        <p className="text-sm text-muted-foreground">
          Register image source repositories and directories on this server.
        </p>
      </div>
      {!loading && !hasTariboyStore(stores) && <TariboyStoreCard actions={props.actions} />}
      <AddStoreForm {...props} />
      {loading && (
        <p role="status" className="text-sm text-muted-foreground">
          Loading Stores…
        </p>
      )}
      {!loading && <StoresTable basePath={props.basePath} stores={stores} />}
    </>
  );
}
