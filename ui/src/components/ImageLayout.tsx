import { useEffect, useState } from "react";
import {
  Link, NavLink, Outlet, useNavigate, useOutletContext, useParams,
} from "react-router-dom";
import { toast } from "sonner";
import {
  ApiError, imageManifestGet, imageProvenanceGet, listImages, removeImage,
  type ImageManifest, type ImageProvenance, type ImageRow,
} from "@/lib/api";
import { openHostPathInVSCode } from "@/lib/desktop";
import { canOpenAgentCwdInVSCode } from "@/pages/agents/agentCwdVSCode";
import { useOptionalDaemons } from "@/components/DaemonProvider";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";

export interface ImageOutletContext {
  ref: string;
  manifest: ImageManifest | null;
  hostKey: string;
  provenance: ImageProvenance | null;
}

export function useImageContext() {
  return useOutletContext<ImageOutletContext>();
}

const TABS = [
  { to: ".", label: "Overview", end: true },
  { to: "template", label: "Template", end: false },
  { to: "skills", label: "Skills", end: false },
  { to: "files", label: "Files", end: false },
];

function message(error: unknown): string {
  return error instanceof ApiError ? error.message : String(error);
}

export function ImageLayout({ hostId, basePath = "/images" }: {
  hostId?: string;
  basePath?: string;
}) {
  const { name = "", tag = "" } = useParams();
  const ref = `${name}:${tag}`;
  const navigate = useNavigate();
  const daemonContext=useOptionalDaemons();
  const hostKey = hostId ?? daemonContext?.activeId ?? "";
  const [manifest, setManifest] = useState<ImageManifest | null>(null);
  const [row, setRow] = useState<ImageRow | null>(null);
  const [provenance,setProvenance]=useState<ImageProvenance|null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let alive = true;
    void Promise.all([imageManifestGet(ref), listImages(),imageProvenanceGet(ref)])
      .then(([nextManifest, listing,nextProvenance]) => {
        if (!alive) return;
        setManifest(nextManifest);
        setRow(
          (listing.images ?? []).find((image) => `${image.name}:${image.tag}` === ref) ?? null,
        );
        setProvenance(nextProvenance);
        setError("");
      })
      .catch((err) => {
        if (alive) setError(message(err));
      });
    return () => { alive = false; };
  }, [hostKey, ref]);

  const bare = row?.bare ?? manifest?.bare ?? false;

  const remove = async () => {
    try {
      await removeImage(ref);
      toast.success(`image ${ref} removed`);
      navigate(`${basePath}?tab=built`);
    } catch (err) {
      toast.error(`remove failed: ${message(err)}`);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <header className="flex flex-wrap items-center gap-3 border-b px-4 py-2">
        <span className="font-mono text-sm font-medium">{ref}</span>
        {bare && <Badge variant="secondary">Terminal-only</Badge>}
        {manifest?.digest && (
          <span className="font-mono text-xs text-muted-foreground">{manifest.digest.slice(0, 16)}</span>
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
          {provenance?.source_cwd
            ? <span className="font-mono text-xs" title={provenance.source_cwd}>{provenance.source_cwd}</span>
            : <span className="text-xs text-muted-foreground">source provenance unavailable</span>}
          {provenance?.source_cwd && provenance.source_available && canOpenAgentCwdInVSCode(hostKey, daemonContext?.daemons ?? []) && (
            <Button size="sm" variant="outline" onClick={() => {
              void openHostPathInVSCode(hostKey, provenance.source_cwd!).catch((error) => toast.error(`Open in VS Code failed: ${message(error)}`));
            }}>Open in VS Code</Button>
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
                    Deletes this immutable runnable image. Original build files are not managed by Tariboy.
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
                isActive
                  ? "border-b-2 border-primary font-medium"
                  : "text-muted-foreground",
              )
            }
          >
            {tabItem.label}
          </NavLink>
        ))}
      </nav>
      <div className="flex-1 overflow-auto p-4">
        {error ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : (
          <Outlet context={{ ref, manifest, hostKey,provenance } satisfies ImageOutletContext} />
        )}
      </div>
    </div>
  );
}
