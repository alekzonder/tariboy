import { useState } from "react";
import { toast } from "sonner";
import type { ApiClient } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { errorText } from "@/lib/errors";
import { splitImageRef } from "./builtImages";

interface PendingImport {
  id: string;
  ref: string;
  name: string;
  tag: string;
  api: ApiClient;
  hostId: string;
}

function ImportFields({
  imageImport,
  onChange,
  onImported,
}: {
  imageImport: PendingImport;
  onChange: (next: PendingImport | null) => void;
  onImported: () => void;
}) {
  return (
    <div className="mt-2 grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
      <label className="text-xs">
        Import name
        <Input
          aria-label="Import name"
          value={imageImport.name}
          onChange={(event) => onChange({ ...imageImport, name: event.target.value })}
        />
      </label>
      <label className="text-xs">
        Import tag
        <Input
          aria-label="Import tag"
          value={imageImport.tag}
          onChange={(event) => onChange({ ...imageImport, tag: event.target.value })}
        />
      </label>
      <Button
        size="sm"
        disabled={!imageImport.name.trim() || !imageImport.tag.trim()}
        onClick={() =>
          void imageImport.api.images
            .applyImageArchive(
              imageImport.id,
              `${imageImport.name.trim()}:${imageImport.tag.trim()}`,
            )
            .then(() => {
              toast.success("image imported");
              onChange(null);
              onImported();
            })
            .catch((error) => toast.error(`import failed: ${errorText(error)}`))
        }
      >
        Import image
      </Button>
    </div>
  );
}

export function ImageArchiveImport({
  hostId,
  onImported,
}: {
  hostId: string;
  onImported: () => void;
}) {
  const api = useApi();
  const [imageImport, setImageImport] = useState<PendingImport | null>(null);

  return (
    <div className="rounded border p-3">
      <label htmlFor="image-archive" className="text-sm font-medium">
        Import runnable image
      </label>
      <Input
        id="image-archive"
        aria-label="Import image archive"
        type="file"
        accept=".gz,.tgz,application/gzip"
        className="mt-1"
        onChange={(event) => {
          const file = event.target.files?.[0];
          if (!file) return;
          const source = api;
          void source.images
            .uploadImageArchive(file)
            .then((preview) => {
              setImageImport({
                id: preview.import_id,
                ref: preview.ref,
                ...splitImageRef(preview.ref),
                api: source,
                hostId,
              });
            })
            .catch((error) => toast.error(`import failed: ${errorText(error)}`));
        }}
      />
      {imageImport?.hostId === hostId && (
        <ImportFields imageImport={imageImport} onChange={setImageImport} onImported={onImported} />
      )}
    </div>
  );
}
