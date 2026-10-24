import { useCallback, useEffect, useRef, useState } from "react";
import { Link, NavLink, Outlet, useLocation, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  ApiError,
  createApi,
  type ApiClient,
  type ImageManifest,
  type ImageProvenance,
} from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import { ApiProvider } from "@/components/ApiProvider";
import { LazyBoundary } from "@/components/LazyBoundary";
import { openHostPathInVSCode } from "@/lib/desktop";
import { canOpenAgentCwdInVSCode } from "@/pages/agents/agentCwdVSCode";
import { useOptionalDaemons } from "@/components/daemonContext";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { cn } from "@/lib/utils";
import type { ImageOutletContext } from "./imageContext";

const TABS = [
  { to: ".", label: "Overview", end: true },
  { to: "template", label: "Template", end: false },
  { to: "skills", label: "Skills", end: false },
  { to: "files", label: "Files", end: false },
];

function message(error: unknown): string {
  return error instanceof ApiError ? error.message : String(error);
}

export function ImageLayout({
  hostId,
  basePath = "/images",
}: {
  hostId?: string;
  basePath?: string;
}) {
  const { name = "", tag = "" } = useParams();
  const hostKey = hostId ?? "";
  return (
    <ImageDetail
      key={`${hostKey}:${name}:${tag}`}
      hostKey={hostKey}
      name={name}
      tag={tag}
      basePath={basePath}
    />
  );
}

function ImageDetail({
  hostKey,
  name,
  tag,
  basePath,
}: {
  hostKey: string;
  name: string;
  tag: string;
  basePath: string;
}) {
  const ref = `${name}:${tag}`;
  const navigate = useNavigate();
  const location = useLocation();
  const daemonContext = useOptionalDaemons();
  const [detail, setDetail] = useState<{
    manifest: ImageManifest;
    provenance: ImageProvenance;
    api: ApiClient;
  } | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [revision, setRevision] = useState(0);
  const refreshes = useRef(0);
  const onReadError = useCallback((cause: unknown) => {
    setDetail(null);
    if (cause instanceof ApiError && cause.code === "image_changed") {
      setNotice("Image changed; refreshing the displayed build.");
      if (refreshes.current++ === 0) {
        setRevision((value) => value + 1);
        return;
      }
    }
    setError(message(cause));
  }, []);

  useEffect(() => {
    let alive = true;
    void (async () => {
      const target = await resolveDaemon(hostKey);
      if (hostKey && !target) throw new Error(`host ${hostKey} is not available`);
      // The image's host is resolved here, so this layout is the server
      // boundary of its tabs: they read through the same client.
      const api = createApi(target);
      const manifest = await api.images.imageManifestGet(ref);
      const provenance = await api.images.imageProvenanceGet(ref, manifest.digest);
      if (provenance.digest && provenance.digest !== manifest.digest)
        throw new ApiError(409, "image_changed", "Image changed; refresh to inspect the new build");
      if (alive) {
        setDetail({ manifest, provenance, api });
        setError("");
      }
    })().catch((cause) => {
      if (alive) onReadError(cause);
    });
    return () => {
      alive = false;
    };
  }, [hostKey, ref, revision, onReadError]);

  const manifest = detail?.manifest;
  const provenance = detail?.provenance;
  const bare = manifest?.bare ?? false;
  const remove = async () => {
    try {
      if (!detail) return;
      await detail.api.images.removeImage(ref);
      toast.success(`image ${ref} removed`);
      navigate(`${basePath}/${encodeURIComponent(name)}`);
    } catch (err) {
      toast.error(`remove failed: ${message(err)}`);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <nav aria-label="Image breadcrumbs" className="flex gap-2 border-b px-4 py-2 text-sm">
        <Link className="text-primary hover:underline" to={basePath}>
          Images
        </Link>
        <span>/</span>
        <Link
          className="text-primary hover:underline"
          to={`${basePath}/${encodeURIComponent(name)}`}
        >
          {name}
        </Link>
        <span>/</span>
        <span>{tag}</span>
      </nav>
      {notice && (
        <p role="status" className="px-4 py-2 text-sm text-muted-foreground">
          {notice}
        </p>
      )}
      <header className="flex flex-wrap items-center gap-3 border-b px-4 py-2">
        <span className="font-mono text-sm font-medium">{ref}</span>
        {manifest && (
          <span className="text-sm">{manifest.image_version || "Version not specified"}</span>
        )}
        {bare && <Badge variant="secondary">Terminal-only</Badge>}
        {manifest?.digest && (
          <span className="font-mono text-xs text-muted-foreground">
            {manifest.digest.slice(0, 16)}
          </span>
        )}
        {manifest?.built_at && (
          <span className="text-xs text-muted-foreground">built {manifest.built_at}</span>
        )}
        <div className="ml-auto flex items-center gap-1">
          <Button size="sm" variant="outline" asChild>
            <Link
              to={`/?new=1&host=${encodeURIComponent(hostKey)}&image=${encodeURIComponent(ref)}`}
            >
              Run Agent
            </Link>
          </Button>
          {provenance?.source_cwd ? (
            <span className="font-mono text-xs" title={provenance.source_cwd}>
              {provenance.source_cwd}
            </span>
          ) : (
            <span className="text-xs text-muted-foreground">source provenance unavailable</span>
          )}
          {provenance?.source_cwd &&
            provenance.source_available &&
            canOpenAgentCwdInVSCode(hostKey, daemonContext?.daemons ?? []) && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  void openHostPathInVSCode(hostKey, provenance.source_cwd!).catch((error) =>
                    toast.error(`Open in VS Code failed: ${message(error)}`),
                  );
                }}
              >
                Open in VS Code
              </Button>
            )}
          {!bare && manifest && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button size="sm" variant="destructive" aria-label={`Remove ${ref}`}>
                  Remove
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Remove image {ref}?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Deletes this runnable image tag. Original build files are not managed by
                    Tariboy.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction variant="destructive" onClick={() => void remove()}>
                    Remove image
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
        </div>
      </header>
      <nav className="flex items-center gap-1 border-b px-2">
        {TABS.map((tabItem) => (
          <NavLink
            key={tabItem.to}
            to={tabItem.to}
            end={tabItem.end}
            className={({ isActive }) =>
              cn(
                "px-3 py-2 text-sm",
                isActive ? "border-b-2 border-primary font-medium" : "text-muted-foreground",
              )
            }
          >
            {tabItem.label}
          </NavLink>
        ))}
      </nav>
      <div className="flex-1 overflow-auto p-4">
        {error ? (
          <div className="space-y-2">
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
            <Button
              variant="outline"
              onClick={() => {
                refreshes.current = 0;
                setError("");
                setRevision((value) => value + 1);
              }}
            >
              Refresh image
            </Button>
          </div>
        ) : detail ? (
          <ApiProvider client={detail.api}>
            <LazyBoundary resetKey={location.pathname}>
              <Outlet
                key={detail.manifest.digest}
                context={
                  {
                    ref,
                    manifest: detail.manifest,
                    hostKey,
                    provenance: detail.provenance,
                    onReadError,
                  } satisfies ImageOutletContext
                }
              />
            </LazyBoundary>
          </ApiProvider>
        ) : (
          <p className="text-sm text-muted-foreground">Loading…</p>
        )}
      </div>
    </div>
  );
}
