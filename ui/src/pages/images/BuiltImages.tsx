import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { ApiError, listImagesOn, removeImage, type ImageRow } from "@/lib/api";
import { useOptionalDaemons } from "@/components/DaemonProvider";
import { resolveDaemon, type Daemon } from "@/lib/daemons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  applyImageArchiveOn, downloadImageArchiveOn, uploadImageArchiveOn,
} from "@/lib/teamApi";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { ImageTransferDialog } from "./ImageTransferDialog";

function message(error: unknown): string {
  return error instanceof ApiError ? error.message : String(error);
}

export default function BuiltImages({ hostId, basePath = "/images" }: {
  hostId: string;
  basePath?: string;
}) {
  const daemonContext = useOptionalDaemons();
  const [images, setImages] = useState<ImageRow[]>([]);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [source, setSource] = useState<{ hostId: string; target: Daemon | null } | null>(null);
  const registryDaemons = useMemo(() => daemonContext?.daemons ?? [], [daemonContext?.daemons]);
  const registryGeneration = useMemo(() => JSON.stringify(registryDaemons.map((daemon) => ({
    id: daemon.id,
    label: daemon.label,
    baseURL: daemon.baseURL,
    state: daemon.state,
  }))), [registryDaemons]);
  const [resolvedTransferDaemons, setResolvedTransferDaemons] = useState<{
    generation: string;
    daemons: Daemon[];
  }>({ generation: "", daemons: [] });
  const [transfer, setTransfer] = useState<{ ref: string; sourceTarget: Daemon | null; hostId: string } | null>(null);
  const [imageImport, setImageImport] = useState<{ id: string; ref: string; name: string; tag: string; target: Daemon | null; hostId: string } | null>(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);

  useEffect(() => {
    let alive = true;
    void resolveDaemon(hostId)
      .then((target) => {
        if (!alive) return;
        if (hostId && !target) {
          setError(`host ${hostId} is not available`);
          return;
        }
        setSource({ hostId, target });
      })
      .catch((err) => {
        if (alive) setError(message(err));
      });
    return () => { alive = false; };
  }, [hostId]);

  useEffect(() => {
    let alive = true;
    void Promise.allSettled(registryDaemons.map((daemon) => resolveDaemon(daemon.id))).then((results) => {
      if (!alive) return;
      setResolvedTransferDaemons({
        generation: registryGeneration,
        daemons: results.flatMap((result, index) => result.status === "fulfilled" && result.value
          ? [{ ...registryDaemons[index], ...result.value }]
          : []),
      });
    });
    return () => { alive = false; };
  }, [registryDaemons, registryGeneration]);

  useEffect(() => {
    const refresh = () => setRevision((value) => value + 1);
    window.addEventListener("tariboy:image-built", refresh);
    return () => window.removeEventListener("tariboy:image-built", refresh);
  }, []);

  useEffect(() => {
    if (source?.hostId !== hostId) return;
    let alive = true;
    void listImagesOn(source.target)
      .then((result) => {
        if (alive) {
          setImages(result.images ?? []);
          setError("");
        }
      })
      .catch((err) => {
        if (alive) setError(message(err));
      });
    return () => { alive = false; };
  }, [hostId, revision, source]);

  const sourceReady = source?.hostId === hostId;
  const transferDaemonsReady = resolvedTransferDaemons.generation === registryGeneration;

  const remove = async (ref: string) => {
    if (source?.hostId !== hostId) return;
    try {
      await removeImage(ref, source.target);
      setImages((current) => current.filter((image) => `${image.name}:${image.tag}` !== ref));
      toast.success(`image ${ref} removed`);
    } catch (error) {
      toast.error(`remove failed: ${message(error)}`);
    }
  };

  const saveArchive = async (ref: string) => {
    if (source?.hostId !== hostId) return;
    try {
      const blob = await downloadImageArchiveOn(source.target, ref);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      const separator = ref.lastIndexOf(":");
      const name = ref.slice(0, separator).replaceAll("/", "_");
      const tag = ref.slice(separator + 1);
      const filename = `${name}-${tag}.tariboy-image.tar.gz`;
      link.href = url; link.download = filename; link.click();
      URL.revokeObjectURL(url);
      toast.success(`image ${ref} saved to file ${filename}`);
    } catch (error) { toast.error(`export failed: ${message(error)}`); }
  };

  if (error) return <p className="text-sm text-destructive">{error}</p>;
  return (
    <div className="space-y-3">
      <div className="rounded border p-3">
        <label htmlFor="image-archive" className="text-sm font-medium">Import runnable image</label>
        <Input id="image-archive" aria-label="Import image archive" type="file" accept=".gz,.tgz,application/gzip" className="mt-1"
          disabled={!sourceReady}
          onChange={(event) => { const file = event.target.files?.[0]; if (!file || source?.hostId !== hostId) return; const target = source.target; void uploadImageArchiveOn(target, file).then((preview) => { const separator = preview.ref.lastIndexOf(":"); setImageImport({ id: preview.import_id, ref: preview.ref, name: preview.ref.slice(0, separator), tag: preview.ref.slice(separator + 1), target, hostId }); }).catch((error) => toast.error(`import failed: ${message(error)}`)); }} />
        {imageImport?.hostId === hostId && <div className="mt-2 grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
          <label className="text-xs">Import name
            <Input aria-label="Import name" value={imageImport.name} onChange={(event) => setImageImport({ ...imageImport, name: event.target.value })} />
          </label>
          <label className="text-xs">Import tag
            <Input aria-label="Import tag" value={imageImport.tag} onChange={(event) => setImageImport({ ...imageImport, tag: event.target.value })} />
          </label>
          <Button size="sm" disabled={!imageImport.name.trim() || !imageImport.tag.trim()} onClick={() => void applyImageArchiveOn(imageImport.target, imageImport.id, `${imageImport.name.trim()}:${imageImport.tag.trim()}`).then(() => { toast.success("image imported"); setImageImport(null); setRevision((value) => value + 1); }).catch((error) => toast.error(`import failed: ${message(error)}`))}>Import image</Button>
        </div>}
      </div>
      <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2">Image</th>
            <th className="px-3 py-2">Digest</th>
            <th className="px-3 py-2">Built</th>
            <th className="px-3 py-2">Source</th>
            <th className="px-3 py-2" />
          </tr>
        </thead>
        <tbody>
          {images.map((image) => {
            const ref = `${image.name}:${image.tag}`;
            return (
              <tr key={ref} data-testid={`built-image-${ref}`} className="border-t">
                <td className="px-3 py-2">
                  <div className="flex items-center gap-2">
                    <Link
                      className="font-mono text-primary hover:underline"
                      to={`${basePath}/${encodeURIComponent(image.name)}/${encodeURIComponent(image.tag)}`}
                    >
                      {ref}
                    </Link>
                    {image.bare && <Badge variant="secondary">Terminal-only</Badge>}
                  </div>
                  {(image.current_agents?.length ?? 0) > 0 && (
                    <div className="mt-1 text-xs text-muted-foreground">
                      Current: {image.current_agents!.join(", ")}
                    </div>
                  )}
                  {(image.pending_agents?.length ?? 0) > 0 && (
                    <div className="text-xs text-muted-foreground">
                      Pending: {image.pending_agents!.join(", ")}
                    </div>
                  )}
                </td>
                <td className="px-3 py-2 font-mono text-xs text-muted-foreground">
                  {image.digest?.slice(0, 18) ?? "—"}
                </td>
                <td className="px-3 py-2 text-xs text-muted-foreground">{image.built_at ?? "—"}</td>
                <td className="px-3 py-2 font-mono text-xs break-all">
                  {!image.source_cwd
                    ? "Source CWD unavailable — imported artifact"
                    : image.source_available === false
                      ? `Source CWD unavailable — ${image.source_cwd}`
                      : image.source_cwd}
                </td>
                <td className="px-3 py-2">
                  <div className="flex justify-end gap-1">
                    <Button size="sm" variant="outline" asChild>
                      <Link
                        to={`/?new=1&host=${encodeURIComponent(hostId)}&image=${encodeURIComponent(ref)}`}
                      >
                        Run Agent
                      </Link>
                    </Button>
                    {!image.bare && (
                      <span title="Export runnable image artifact (original sources are not included)">
                        <Button size="sm" variant="outline" disabled={!image.exportable || !sourceReady}
                          aria-label={`Export ${ref}`} onClick={() => void saveArchive(ref)}>Export</Button>
                      </span>
                    )}
                    {!image.bare && image.exportable && sourceReady && (
                      <Button size="sm" variant="outline" aria-label={`Upload to servers ${ref}`}
                        disabled={!transferDaemonsReady}
                        title={transferDaemonsReady ? undefined : "Loading transfer servers"}
                        onClick={() => {
                          if (transferDaemonsReady) setTransfer({ ref, sourceTarget: source.target, hostId });
                        }}>Upload to servers</Button>
                    )}
                    {!image.bare && (
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            size="sm"
                            variant="destructive"
                            aria-label={`Remove ${ref}`}
                            disabled={!sourceReady}
                          >
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
                            <AlertDialogAction
                              variant="destructive"
                              onClick={() => void remove(ref)}
                            >
                              Remove image
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    )}
                  </div>
                </td>
              </tr>
            );
          })}
          {images.length === 0 && (
            <tr>
              <td colSpan={5} className="px-3 py-8 text-center text-muted-foreground">
                No built images on this host.
              </td>
            </tr>
          )}
        </tbody>
      </table>
      </div>
      {transfer && transfer.hostId === hostId && (
        <ImageTransferDialog
          open
          onOpenChange={(open) => { if (!open) setTransfer(null); }}
          source={transfer.sourceTarget}
          imageRef={transfer.ref}
          daemons={resolvedTransferDaemons.daemons}
          onComplete={() => { if (mounted.current) setRevision((value) => value + 1); }}
        />
      )}
    </div>
  );
}
