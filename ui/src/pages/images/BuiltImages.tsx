import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { type ApiClient, type ImageSummary } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { subscribeImagesChanged } from "@/lib/imageEvents";
import { ImageTransferDialog } from "./ImageTransferDialog";
import { errorText } from "@/lib/errors";
import { archiveFilename } from "./built/builtImages";
import { BuiltImageTagsTable } from "./built/BuiltImageTagsTable";
import { useBuiltImageGroups } from "./built/useBuiltImageGroups";
import { useTransferDaemons } from "./built/useTransferDaemons";

/** How many versions show before "Show all". */
const COLLAPSED_VERSIONS = 5;

/** The Versions of one image name: its tags, newest first, with their
 *  actions. `currentTag` is the tag the page shows. */
export default function BuiltImages({
  hostId,
  imageName,
  currentTag,
}: {
  hostId: string;
  imageName: string;
  currentTag?: string;
}) {
  const [showAll, setShowAll] = useState(false);
  const api = useApi();
  // Images are kept with the client that listed them: rows of a previous
  // route host are never shown (or acted on) for the current one.
  const [listed, setListed] = useState<{ api: ApiClient; images: ImageSummary[] } | null>(null);
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

  const { tags } = useBuiltImageGroups(images, imageName);
  const shown = showAll ? tags : tags.slice(0, COLLAPSED_VERSIONS);

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
    <section aria-label="Versions" className="space-y-2">
      <h2 className="font-medium">Versions</h2>
      <BuiltImageTagsTable
        tags={shown}
        newestTag={tags[0]?.tag}
        currentTag={currentTag}
        actions={{
          hostId,
          transferReady: transferDaemons.ready,
          onExport: (ref) => void saveArchive(ref),
          onTransfer: (ref) => setTransfer({ ref, hostId }),
          onRemove: (ref) => void remove(ref),
        }}
      />
      {shown.length < tags.length && (
        <Button size="xs" variant="outline" onClick={() => setShowAll(true)}>
          Show all {tags.length}
        </Button>
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
    </section>
  );
}
