import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { agentImageCancelOn, agentImageSetOn } from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import { serverPath } from "@/lib/terminalsHost";
import { ActionRow, DraftField, FieldFrame, ReadOnly } from "./ConfigControls";
import { fieldOfKey, type ConfigField } from "./registry";
import type { BlockProps } from "./ConfigurationBlocks";

const field = (key: string) => fieldOfKey(key) as ConfigField;

// A digest is a 64-hex-character hash: shown whole it dominates the identity
// row and wraps across lines. Keep enough of both ends to compare against a
// registry listing by eye; the untruncated value stays in the a11y tree.
function shortDigest(digest: string): string {
  return digest.length > 24 ? `${digest.slice(0, 14)}…${digest.slice(-8)}` : digest;
}

export function GeneralBlock({ draft, matched, sources, target, name, stopped, refresh }: BlockProps) {
  const { view, imageStatus, imageError, images, inventoryLoading, inventoryError } = sources;
  const [copied, setCopied] = useState(false);
  const [imageBusy, setImageBusy] = useState(false);
  const imageHref = (ref: string) => {
    const split = ref.lastIndexOf(":");
    const base = serverPath(target?.id ?? "", "images");
    return split < 0 ? base : `${base}/${encodeURIComponent(ref.slice(0, split))}/${encodeURIComponent(ref.slice(split + 1))}`;
  };
  const imageAction = async (label: string, run: () => Promise<unknown>) => {
    setImageBusy(true);
    await guard(label, async () => {
      await run();
      await sources.loadImages();
      refresh();
    });
    setImageBusy(false);
  };
  const copyDigest = async (digest: string) => {
    try {
      await navigator.clipboard.writeText(digest);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };
  const imageField = field("image");
  const cwdField = field("cwd");
  const cwdError = draft.fieldErrors.cwd ?? "";
  return (
    <>
      {imageError && <p role="alert" className="text-sm text-destructive">Image status unavailable: {imageError}</p>}
      {inventoryError && <p role="alert" className="text-sm text-destructive">Image inventory unavailable: {inventoryError}</p>}
      {imageStatus && (
        <div className="grid gap-4 text-sm sm:grid-cols-2">
          {([{ label: stopped ? "Activated version" : "Current", value: imageStatus.current }, { label: "Next iteration", value: imageStatus.next }] as const).map(({ label, value }) => value && (
            <div key={label} role="group" aria-label={label} className="space-y-0.5">
              <div className="font-medium">{label}</div>
              <Link className="font-mono text-primary hover:underline" to={imageHref(value.ref)}>{value.ref}</Link>
              <div>{value.error ? "Version unavailable" : value.image_version || "Version not specified"}</div>
              <div className="font-mono text-xs break-all">{value.digest || "Digest unavailable"}</div>
              {value.error && <p role="alert" className="mt-1 text-destructive">{value.error}</p>}
            </div>
          ))}
          {imageStatus.next && (
            <div className="space-y-1 sm:col-span-2">
              <p className="text-xs text-muted-foreground">{imageStatus.next.reason === "pending" ? "Explicit pending assignment" : imageStatus.next.reason === "ref_moved" ? "Content the active ref points at now" : "Current pinned image"}</p>
              {!imageStatus.next.error && imageStatus.next.digest && imageStatus.next.digest !== imageStatus.current.digest && <p role="status">Update on next iteration</p>}
              <p className="text-xs text-muted-foreground">This preview is not a reservation; the image may change before launch.</p>
            </div>
          )}
          {imageStatus.pending.ref && <div className="sm:col-span-2">Pending: <Link className="font-mono text-primary hover:underline" to={imageHref(imageStatus.pending.ref)}>{imageStatus.pending.ref}</Link></div>}
          {imageStatus.pending.error && <p role="alert" className="text-destructive sm:col-span-2">{imageStatus.pending.error}</p>}
        </div>
      )}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <FieldFrame field={imageField} draft={draft} matched={matched.has("image")}>
          <Select
            disabled={!imageStatus || inventoryLoading || !!inventoryError}
            value={draft.values.image ?? ""}
            onValueChange={(value) => draft.setField("image", String(value ?? ""))}
          >
            <SelectTrigger id="image" aria-label="Agent image" aria-describedby={draft.fieldErrors.image ? "image-help image-error" : "image-help"} className="w-full">
              <SelectValue placeholder="Select image" />
            </SelectTrigger>
            <SelectContent>
              {images.map((item) => {
                const ref = `${item.name}:${item.tag}`;
                return <SelectItem key={ref} value={ref}>{ref}</SelectItem>;
              })}
            </SelectContent>
          </Select>
        </FieldFrame>
        <ReadOnly draft={draft} matched={matched} fieldKey="image-digest">
          <div className="flex min-h-9 flex-wrap items-center gap-2 text-sm">
            {view?.digest ? (
              <>
                <span data-testid="image-digest" aria-hidden="true" title={view.digest} className="font-mono text-xs break-all">{shortDigest(view.digest)}</span>
                <span className="sr-only">{view.digest}</span>
              </>
            ) : <span>—</span>}
          </div>
        </ReadOnly>
      </div>
      <div className="text-sm">
        <span className="text-muted-foreground">Image</span> <span className="break-all">{view?.image}</span>
      </div>
      {stopped ? (
        <>
          <FieldFrame field={cwdField} draft={draft} matched={matched.has("cwd")}>
            <PathAutocomplete
              id="cwd"
              value={draft.values.cwd ?? ""}
              onChange={(value) => draft.setField("cwd", value)}
              daemon={target}
              aria-label="Working directory"
            />
          </FieldFrame>
          <DraftField field={field("cwd-create")} draft={draft} matched={matched.has("cwd-create")} />
        </>
      ) : (
        <div data-field="cwd" className="space-y-1 text-sm">
          <div className="font-medium">Working directory</div>
          <p className="break-all">{view?.cwd || "—"}</p>
          {cwdError && <p role="alert" className="text-xs text-destructive">{cwdError}</p>}
          <p className="text-xs text-muted-foreground">Stop the agent before changing its working directory.</p>
        </div>
      )}
      {(view?.digest || imageStatus?.pending.ref) && (
        <ActionRow>
          {view?.digest && <Button size="sm" variant="outline" onClick={() => void copyDigest(view.digest)}>Copy digest</Button>}
          {copied && <span role="status" className="text-xs text-muted-foreground">Digest copied</span>}
          {imageStatus?.pending.error && (
            <Button size="sm" variant="outline" disabled={imageBusy} onClick={() => void imageAction("image assignment", () => agentImageSetOn(target, name, imageStatus.pending.ref))}>
              Retry
            </Button>
          )}
          {imageStatus?.pending.ref && (
            <Button size="sm" variant="outline" disabled={imageBusy} onClick={() => void imageAction("pending image cancellation", () => agentImageCancelOn(target, name))}>
              Cancel pending
            </Button>
          )}
        </ActionRow>
      )}
    </>
  );
}
