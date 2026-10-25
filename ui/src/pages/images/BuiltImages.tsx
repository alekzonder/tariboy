import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { type ApiClient, type ImageRow } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { subscribeImagesChanged } from "@/lib/imageEvents";
import { ImageTransferDialog } from "./ImageTransferDialog";
import { errorText } from "@/lib/errors";
import { archiveFilename } from "./built/builtImages";
import { BuiltImageGroupsTable } from "./built/BuiltImageGroupsTable";
import { BuiltImageTagsTable } from "./built/BuiltImageTagsTable";
import { ImageArchiveImport } from "./built/ImageArchiveImport";
import { useBuiltImageGroups } from "./built/useBuiltImageGroups";
import { useTransferDaemons } from "./built/useTransferDaemons";

export default function BuiltImages({
  hostId,
  basePath = "/images",
  imageName,
}: {
  hostId: string;
  basePath?: string;
  imageName?: string;
}) {
  const api = useApi();
  // Images are kept with the client that listed them: rows of a previous
  // route host are never shown (or acted on) for the current one.
  const [listed, setListed] = useState<{ api: ApiClient; images: ImageRow[] } | null>(null);
  const images = useMemo(() => (listed?.api === api ? listed.images : []), [api, listed]);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const transferDaemons = useTransferDaemons();
  const [transfer, setTransfer] = useState<{ ref: string; hostId: string } | null>(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => subscribeImagesChanged(() => setRevision((value) => value + 1)), []);

  useEffect(() => {
    let alive = true;
    void api.images
      .listImages()
      .then((result) => {
        if (alive) {
          setListed({ api, images: result.images ?? [] });
          setError("");
        }
      })
      .catch((err) => {
        if (alive) setError(errorText(err));
      });
    return () => {
      alive = false;
    };
  }, [api, revision]);

  const { groups, tags } = useBuiltImageGroups(images, imageName);

  const remove = async (ref: string) => {
    const source = api;
    try {
      await source.images.removeImage(ref);
      setListed((current) =>
        current?.api === source
          ? {
              ...current,
              images: current.images.filter((image) => `${image.name}:${image.tag}` !== ref),
            }
          : current,
      );
      toast.success(`image ${ref} removed`);
    } catch (error) {
      toast.error(`remove failed: ${errorText(error)}`);
    }
  };

  const saveArchive = async (ref: string) => {
    try {
      const blob = await api.images.downloadImageArchive(ref);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      const filename = archiveFilename(ref);
      link.href = url;
      link.download = filename;
      link.click();
      URL.revokeObjectURL(url);
      toast.success(`image ${ref} saved to file ${filename}`);
    } catch (error) {
      toast.error(`export failed: ${errorText(error)}`);
    }
  };

  if (error) return <p className="text-sm text-destructive">{error}</p>;
  return (
    <div className="space-y-3">
      {imageName === undefined && (
        <ImageArchiveImport hostId={hostId} onImported={() => setRevision((value) => value + 1)} />
      )}
      {imageName === undefined ? (
        <BuiltImageGroupsTable groups={groups} basePath={basePath} />
      ) : (
        <BuiltImageTagsTable
          tags={tags}
          basePath={basePath}
          actions={{
            hostId,
            transferReady: transferDaemons.ready,
            onExport: (ref) => void saveArchive(ref),
            onTransfer: (ref) => setTransfer({ ref, hostId }),
            onRemove: (ref) => void remove(ref),
          }}
        />
      )}
      {transfer && transfer.hostId === hostId && (
        <ImageTransferDialog
          open
          onOpenChange={(open) => {
            if (!open) setTransfer(null);
          }}
          imageRef={transfer.ref}
          daemons={transferDaemons.daemons}
          onComplete={() => {
            if (mounted.current) setRevision((value) => value + 1);
          }}
        />
      )}
    </div>
  );
}
