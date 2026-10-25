import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { DraftField, FieldFrame, ReadOnly } from "./ConfigControls";
import { ImageActions, ImageIdentity } from "./GeneralImage";
import { shortDigest } from "./generalImageModel";
import { fieldOfKey, type ConfigField } from "./registry";
import type { BlockProps } from "./ConfigurationBlocks";

const field = (key: string) => fieldOfKey(key) as ConfigField;

function ImageSelect({
  draft,
  matched,
  sources,
}: Pick<BlockProps, "draft" | "matched" | "sources">) {
  const { imageStatus, images, inventoryLoading, inventoryError } = sources;
  return (
    <FieldFrame field={field("image")} draft={draft} matched={matched.has("image")}>
      <Select
        disabled={!imageStatus || inventoryLoading || !!inventoryError}
        value={draft.values.image ?? ""}
        onValueChange={(value) => draft.setField("image", String(value ?? ""))}
      >
        <SelectTrigger
          id="image"
          aria-label="Agent image"
          aria-describedby={draft.fieldErrors.image ? "image-help image-error" : "image-help"}
          className="w-full"
        >
          <SelectValue placeholder="Select image" />
        </SelectTrigger>
        <SelectContent>
          {images.map((item) => {
            const ref = `${item.name}:${item.tag}`;
            return (
              <SelectItem key={ref} value={ref}>
                {ref}
              </SelectItem>
            );
          })}
        </SelectContent>
      </Select>
    </FieldFrame>
  );
}

function ImageDigest({
  draft,
  matched,
  digest,
}: Pick<BlockProps, "draft" | "matched"> & { digest?: string }) {
  return (
    <ReadOnly draft={draft} matched={matched} fieldKey="image-digest">
      <div className="flex min-h-9 flex-wrap items-center gap-2 text-sm">
        {digest ? (
          <>
            <span
              data-testid="image-digest"
              aria-hidden="true"
              title={digest}
              className="font-mono text-xs break-all"
            >
              {shortDigest(digest)}
            </span>
            <span className="sr-only">{digest}</span>
          </>
        ) : (
          <span>—</span>
        )}
      </div>
    </ReadOnly>
  );
}

// The working directory can change only while the agent is stopped.
function CwdSetting({
  draft,
  matched,
  stopped,
  cwd,
}: Pick<BlockProps, "draft" | "matched" | "stopped"> & { cwd?: string }) {
  const cwdError = draft.fieldErrors.cwd ?? "";
  if (stopped) {
    return (
      <>
        <FieldFrame field={field("cwd")} draft={draft} matched={matched.has("cwd")}>
          <PathAutocomplete
            id="cwd"
            value={draft.values.cwd ?? ""}
            onChange={(value) => draft.setField("cwd", value)}
            aria-label="Working directory"
          />
        </FieldFrame>
        <DraftField field={field("cwd-create")} draft={draft} matched={matched.has("cwd-create")} />
      </>
    );
  }
  return (
    <div data-field="cwd" className="space-y-1 text-sm">
      <div className="font-medium">Working directory</div>
      <p className="break-all">{cwd || "—"}</p>
      {cwdError && (
        <p role="alert" className="text-xs text-destructive">
          {cwdError}
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        Stop the agent before changing its working directory.
      </p>
    </div>
  );
}

export function GeneralBlock({ draft, matched, sources, name, stopped, refresh }: BlockProps) {
  const { view, imageStatus, imageError, inventoryError } = sources;
  return (
    <>
      {imageError && (
        <p role="alert" className="text-sm text-destructive">
          Image status unavailable: {imageError}
        </p>
      )}
      {inventoryError && (
        <p role="alert" className="text-sm text-destructive">
          Image inventory unavailable: {inventoryError}
        </p>
      )}
      {imageStatus && <ImageIdentity imageStatus={imageStatus} stopped={stopped} />}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <ImageSelect draft={draft} matched={matched} sources={sources} />
        <ImageDigest draft={draft} matched={matched} digest={view?.digest} />
      </div>
      <div className="text-sm">
        <span className="text-muted-foreground">Image</span>{" "}
        <span className="break-all">{view?.image}</span>
      </div>
      <CwdSetting draft={draft} matched={matched} stopped={stopped} cwd={view?.cwd} />
      <ImageActions
        view={view}
        imageStatus={imageStatus}
        sources={sources}
        name={name}
        refresh={refresh}
      />
    </>
  );
}
