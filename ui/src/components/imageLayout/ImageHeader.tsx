import { Link } from "react-router-dom";
import { toast } from "sonner";
import type { ImageManifest, ImageProvenance } from "@/lib/api";
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
import { errorText } from "@/lib/errors";

function ImageSourceInfo({
  hostKey,
  provenance,
}: {
  hostKey: string;
  provenance: ImageProvenance | undefined;
}) {
  const daemonContext = useOptionalDaemons();
  const sourceCwd = provenance?.source_cwd;
  return (
    <>
      {sourceCwd ? (
        <span className="font-mono text-xs" title={sourceCwd}>
          {sourceCwd}
        </span>
      ) : (
        <span className="text-xs text-muted-foreground">source provenance unavailable</span>
      )}
      {sourceCwd &&
        provenance.source_available &&
        canOpenAgentCwdInVSCode(hostKey, daemonContext?.daemons ?? []) && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              void openHostPathInVSCode(hostKey, sourceCwd).catch((error) =>
                toast.error(`Open in VS Code failed: ${errorText(error)}`),
              );
            }}
          >
            Open in VS Code
          </Button>
        )}
    </>
  );
}

function RemoveImageButton({ imageRef, onRemove }: { imageRef: string; onRemove: () => void }) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant="destructive" aria-label={`Remove ${imageRef}`}>
          Remove
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove image {imageRef}?</AlertDialogTitle>
          <AlertDialogDescription>
            Deletes this runnable image tag. Original build files are not managed by Tariboy.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onRemove}>
            Remove image
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function ImageHeader({
  imageRef,
  hostKey,
  manifest,
  provenance,
  onRemove,
}: {
  imageRef: string;
  hostKey: string;
  manifest: ImageManifest | undefined;
  provenance: ImageProvenance | undefined;
  onRemove: () => void;
}) {
  const bare = manifest?.bare ?? false;
  return (
    <header className="flex flex-wrap items-center gap-3 border-b px-4 py-2">
      <span className="font-mono text-sm font-medium">{imageRef}</span>
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
            to={`/?new=1&host=${encodeURIComponent(hostKey)}&image=${encodeURIComponent(imageRef)}`}
          >
            Run Agent
          </Link>
        </Button>
        <ImageSourceInfo hostKey={hostKey} provenance={provenance} />
        {!bare && manifest && <RemoveImageButton imageRef={imageRef} onRemove={onRemove} />}
      </div>
    </header>
  );
}
