import { Link } from "react-router-dom";
import type { OnboardingDefaultStore, OnboardingStatus, StoreView } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useApi } from "@/components/apiContext";
import { GettingReady } from "@/components/onboarding/GettingReady";
import { useOnboardingStatus } from "@/hooks/useOnboardingStatus";
import { checklistRows } from "@/lib/onboarding/firstRun";
import type { StoreActions } from "./useStoreActions";
import { hasTariboyStore } from "./storeStatus";

interface StoreListProps {
  basePath: string;
  stores: StoreView[];
  loading: boolean;
  actions: StoreActions;
  storeName: string;
  setStoreName: (name: string) => void;
  source: string;
  setSource: (source: string) => void;
}

function TariboyStoreCard({
  actions,
  store,
}: Pick<StoreListProps, "actions"> & { store: OnboardingDefaultStore }) {
  return (
    <section className="flex flex-wrap items-center justify-between gap-3 rounded border p-4">
      <div className="min-w-0 space-y-1">
        <h2 className="font-medium">Tariboy Store</h2>
        <p className="break-all text-sm text-muted-foreground">
          Images and workflows published by Tariboy, registered as{" "}
          <span className="font-mono">{store.name}</span> from{" "}
          <span className="font-mono">{store.source}</span>.
        </p>
      </div>
      <Button
        disabled={Boolean(actions.busy)}
        onClick={() => void actions.add({ name: store.name, source: store.source }, "add-tariboy")}
      >
        {actions.busy === "add-tariboy" ? "Adding…" : "Add tariboy Store"}
      </Button>
    </section>
  );
}

function AddStoreForm({ actions, storeName, setStoreName, source, setSource }: StoreListProps) {
  const { busy } = actions;
  return (
    <details className="rounded border p-4">
      <summary className="cursor-pointer font-medium">Add another catalog</summary>
      <div className="mt-3 grid gap-3 md:grid-cols-[14rem_minmax(0,1fr)_auto]">
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
    </details>
  );
}

// The checklist is shown while the host still sets up the catalog or the
// starter image; the coding tool row alone does not concern this page.
const setupPending = (status: OnboardingStatus) =>
  checklistRows(status).some((row) => row.key !== "tool" && row.state !== "done");

// The card offers the default Store the daemon reports, also after it was
// removed. A daemon without the onboarding status offers none, and neither
// does one that is connecting it right now.
const offeredStore = (status: OnboardingStatus | null, stores: StoreView[]) => {
  const store = status?.default_store;
  if (!store || store.state === "pending" || hasTariboyStore(stores, store)) return null;
  return store;
};

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
  const onboarding = useOnboardingStatus(useApi());
  const { status } = onboarding;
  const offered = loading ? null : offeredStore(status, stores);
  return (
    <>
      <div>
        <h1 className="text-lg font-semibold">Stores</h1>
        <p className="text-sm text-muted-foreground">
          Register image source repositories and directories on this server.
        </p>
      </div>
      {status && setupPending(status) && <GettingReady status={status} retry={onboarding.retry} />}
      {offered && <TariboyStoreCard actions={props.actions} store={offered} />}
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
