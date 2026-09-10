import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ApiError } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import {
  addStore,
  buildStoreImage,
  getStore,
  listStores,
  refreshStore,
  removeStore,
  type Store,
  type StoreDetail,
} from "@/lib/stores";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

const message = (error: unknown) => error instanceof ApiError ? error.message : String(error);

export default function StoresPage({ target, name, basePath }: {
  target: Daemon | null;
  name?: string;
  basePath: string;
}) {
  const navigate = useNavigate();
  const mounted = useRef(true);
  const [stores, setStores] = useState<Store[]>([]);
  const [detail, setDetail] = useState<StoreDetail | null>(null);
  const [storeName, setStoreName] = useState("");
  const [source, setSource] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const [removeOpen, setRemoveOpen] = useState(false);
  const [removeError, setRemoveError] = useState("");

  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);

  useEffect(() => {
    let current = true;
    const request = name ? getStore(target, name) : listStores(target);
    void request.then((result) => {
      if (!current) return;
      if (name) setDetail(result as StoreDetail);
      else setStores(result as Store[]);
    }).catch((cause) => {
      if (current) setError(message(cause));
    }).finally(() => {
      if (current) setLoading(false);
    });
    return () => { current = false; };
  }, [name, target]);

  const add = async () => {
    const input = { name: storeName.trim(), source: source.trim() };
    if (!input.name || !input.source) return;
    setBusy("add");
    setError("");
    setStatus("");
    try {
      const added = await addStore(target, input);
      if (mounted.current) navigate(`${basePath}/${encodeURIComponent(added.name)}`);
    } catch (cause) {
      if (mounted.current) setError(message(cause));
    } finally {
      if (mounted.current) setBusy("");
    }
  };

  const refresh = async () => {
    if (!name) return;
    setBusy("refresh");
    setError("");
    setStatus("");
    try {
      const refreshed = await refreshStore(target, name);
      if (mounted.current) {
        setDetail(refreshed);
        setStatus(`Refreshed ${name}.`);
      }
    } catch (cause) {
      if (mounted.current) setError(message(cause));
    } finally {
      if (mounted.current) setBusy("");
    }
  };

  const build = async (imageName: string) => {
    if (!name) return;
    setBusy(`build:${imageName}`);
    setError("");
    setStatus("");
    try {
      const built = await buildStoreImage(target, `${name}/${imageName}`);
      if (mounted.current) {
        setStatus(`Built ${built.name}:${built.tag}.`);
        window.dispatchEvent(new Event("tariboy:image-built"));
      }
      try {
        const refreshed = await getStore(target, name);
        if (mounted.current) setDetail(refreshed);
      } catch (cause) {
        if (mounted.current) setError(message(cause));
      }
    } catch (cause) {
      if (mounted.current) setError(message(cause));
    } finally {
      if (mounted.current) setBusy("");
    }
  };

  const remove = async () => {
    if (!name) return;
    setBusy("remove");
    setError("");
    setRemoveError("");
    setStatus("");
    try {
      await removeStore(target, name);
      if (mounted.current) navigate(basePath);
    } catch (cause) {
      if (mounted.current) {
        const failure = message(cause);
        setError(failure);
        setRemoveError(failure);
      }
    } finally {
      if (mounted.current) setBusy("");
    }
  };

  return <div className="h-full min-h-0 space-y-4 overflow-y-auto p-6">
    {name ? <>
      <div className="space-y-1">
        <Link className="text-sm text-primary hover:underline" to={basePath}>← Stores</Link>
        <div className="flex items-start justify-between gap-3">
          <div>
            <h1 className="text-lg font-semibold">{name}</h1>
            {detail && <>
              <p className="break-all text-sm text-muted-foreground">{detail.source}</p>
              <p className="break-all font-mono text-xs text-muted-foreground">{detail.path}</p>
            </>}
          </div>
          <div className="flex shrink-0 gap-2">
            <Button variant="outline" disabled={Boolean(busy) || loading} onClick={() => void refresh()}>
              {busy === "refresh" ? "Refreshing…" : "Refresh"}
            </Button>
            <AlertDialog open={removeOpen} onOpenChange={(open) => {
              if (busy === "remove") return;
              if (open) setRemoveError("");
              setRemoveOpen(open);
            }}>
              <AlertDialogTrigger asChild>
                <Button variant="destructive" disabled={Boolean(busy) || loading}>Remove store</Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Remove Store {name}?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Removes this registration and its managed clone. It preserves local sources and built images.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                {removeError && <p role="alert" className="text-sm text-destructive">{removeError}</p>}
                <AlertDialogFooter>
                  <AlertDialogCancel disabled={busy === "remove"}>Cancel</AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    disabled={busy === "remove"}
                    onClick={(event) => { event.preventDefault(); void remove(); }}
                  >
                    {busy === "remove" ? "Removing…" : "Remove store"}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </div>
      </div>
      {loading && <p role="status" className="text-sm text-muted-foreground">Loading Store…</p>}
      {detail && <section className="space-y-2">
        <h2 className="font-medium">Images</h2>
        <div className="overflow-x-auto rounded border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
              <tr><th className="px-3 py-2">Image</th><th className="px-3 py-2">Version</th><th className="px-3 py-2">Built version</th><th className="px-3 py-2">Status</th><th className="px-3 py-2" /></tr>
            </thead>
            <tbody>
              {detail.images.map((image) => <tr key={image.name} className={`border-t ${image.update_needed ? "bg-amber-50 dark:bg-amber-950/30" : ""}`}>
                <td className="px-3 py-2 font-mono">{image.name}</td>
                <td className="px-3 py-2 font-mono text-xs">{image.version}</td>
                <td className="px-3 py-2 font-mono text-xs">{image.built_version || "—"}</td>
                <td className={`px-3 py-2 text-xs ${image.error ? "text-destructive" : "text-muted-foreground"}`}>
                  {image.error ?? (image.update_needed ? "Update needed" : "Ready")}
                </td>
                <td className="px-3 py-2 text-right">
                  {!image.error && <Button
                    size="sm"
                    disabled={Boolean(busy)}
                    onClick={() => void build(image.name)}
                  >
                    {busy === `build:${image.name}` ? `Building ${image.name}…` : `Build ${image.name}`}
                  </Button>}
                </td>
              </tr>)}
              {detail.images.length === 0 && <tr><td colSpan={5} className="px-3 py-8 text-center text-muted-foreground">No images in this Store.</td></tr>}
            </tbody>
          </table>
        </div>
      </section>}
    </> : <>
      <div>
        <h1 className="text-lg font-semibold">Stores</h1>
        <p className="text-sm text-muted-foreground">Register image source repositories and directories on this server.</p>
      </div>
      <section className="space-y-3 rounded border p-4">
        <h2 className="font-medium">Add Store</h2>
        <div className="grid gap-3 md:grid-cols-[14rem_minmax(0,1fr)_auto]">
          <Input aria-label="Store name" required placeholder="name" value={storeName} disabled={Boolean(busy)} onChange={(event) => setStoreName(event.target.value)} />
          <Input aria-label="Store source" required placeholder="Git URL or absolute directory" value={source} disabled={Boolean(busy)} onChange={(event) => setSource(event.target.value)} />
          <Button disabled={Boolean(busy) || !storeName.trim() || !source.trim()} onClick={() => void add()}>
            {busy === "add" ? "Adding…" : "Add store"}
          </Button>
        </div>
      </section>
      {loading && <p role="status" className="text-sm text-muted-foreground">Loading Stores…</p>}
      {!loading && <div className="overflow-x-auto rounded border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs text-muted-foreground"><tr><th className="px-3 py-2">Store</th><th className="px-3 py-2">Source</th></tr></thead>
          <tbody>
            {stores.map((store) => <tr key={store.name} className="border-t">
              <td className="px-3 py-2"><Link className="font-medium text-primary hover:underline" to={`${basePath}/${encodeURIComponent(store.name)}`}>{store.name}</Link></td>
              <td className="break-all px-3 py-2 font-mono text-xs text-muted-foreground">{store.source}</td>
            </tr>)}
            {stores.length === 0 && <tr><td colSpan={2} className="px-3 py-8 text-center text-muted-foreground">No Stores registered on this server.</td></tr>}
          </tbody>
        </table>
      </div>}
    </>}
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    {status && <p role="status" className="text-sm text-muted-foreground">{status}</p>}
  </div>;
}
