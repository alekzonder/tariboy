import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { FieldFrame, ReadOnly, type SectionProps } from "./ConfigControls";
import { ImageActions, PendingImage } from "./GeneralImage";
import { shortDigest } from "./generalImageModel";
import { fieldOfKey, type ConfigField } from "./registry";

const field = (key: string) => fieldOfKey(key) as ConfigField;

function ImageSelect({ draft, sources }: Pick<SectionProps, "draft" | "sources">) {
  const { imageStatus, images, inventoryLoading, inventoryError } = sources;
  return (
    <FieldFrame field={field("image")} draft={draft}>
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

function ImageDigest({ draft, digest }: Pick<SectionProps, "draft"> & { digest?: string }) {
  return (
    <ReadOnly draft={draft} fieldKey="image-digest">
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

// GeneralBlock is Advanced → Image: the image choice, the digest once, a
// pending assignment and the image actions.
export function GeneralBlock({ draft, sources, name, refresh }: SectionProps) {
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
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <ImageSelect draft={draft} sources={sources} />
        <ImageDigest draft={draft} digest={view?.digest} />
      </div>
      {imageStatus && <PendingImage imageStatus={imageStatus} />}
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
