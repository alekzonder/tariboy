import { Link } from "react-router-dom";
import type { ImageSummary } from "@/lib/api";
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

export interface TagActionHandlers {
  hostId: string;
  transferReady: boolean;
  onExport: (ref: string) => void;
  onTransfer: (ref: string) => void;
  onRemove: (ref: string) => void;
}

function RemoveImageAction({
  image,
  imageRef,
  onRemove,
}: {
  image: ImageSummary;
  imageRef: string;
  onRemove: (ref: string) => void;
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          size="sm"
          variant="destructive"
          aria-label={`Remove ${imageRef}`}
          disabled={!!image.current_agents?.length || !!image.pending_agents?.length}
        >
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
          <AlertDialogAction variant="destructive" onClick={() => onRemove(imageRef)}>
            Remove image
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function BuiltImageTagActions({
  image,
  imageRef,
  hostId,
  transferReady,
  onExport,
  onTransfer,
  onRemove,
}: TagActionHandlers & { image: ImageSummary; imageRef: string }) {
  return (
    <div className="flex justify-end gap-1">
      <Button size="sm" variant="outline" asChild>
        <Link
          aria-label={`Run agent ${imageRef}`}
          to={`/?new=1&host=${encodeURIComponent(hostId)}&image=${encodeURIComponent(imageRef)}`}
        >
          Run Agent
        </Link>
      </Button>
      {!image.bare && (
        <span title="Export runnable image artifact (original sources are not included)">
          <Button
            size="sm"
            variant="outline"
            disabled={!image.exportable}
            aria-label={`Export ${imageRef}`}
            onClick={() => onExport(imageRef)}
          >
            Export
          </Button>
        </span>
      )}
      {!image.bare && image.exportable && (
        <Button
          size="sm"
          variant="outline"
          aria-label={`Upload to servers ${imageRef}`}
          disabled={!transferReady}
          title={transferReady ? undefined : "Loading transfer servers"}
          onClick={() => {
            if (transferReady) onTransfer(imageRef);
          }}
        >
          Upload to servers
        </Button>
      )}
      {!image.bare && <RemoveImageAction image={image} imageRef={imageRef} onRemove={onRemove} />}
    </div>
  );
}
