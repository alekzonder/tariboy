import type { Dispatch, SetStateAction } from "react";
import type { StoreImage } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { imageStatuses, latestImageVersionLabel } from "./storeStatus";

interface StoreImageRowProps {
  image: StoreImage;
  autoImages: string[];
  setAutoImages: Dispatch<SetStateAction<string[]>>;
  busy: string;
  build: (imageName: string) => Promise<void>;
}

function StoreImageRow({ image, autoImages, setAutoImages, busy, build }: StoreImageRowProps) {
  return (
    <tr className={`border-t ${image.update_needed ? "bg-amber-50 dark:bg-amber-950/30" : ""}`}>
      <td className="px-3 py-2">
        <input
          type="checkbox"
          aria-label={`Automatic builds for ${image.name}`}
          checked={autoImages.includes(image.name)}
          disabled={Boolean(busy) || Boolean(image.error)}
          onChange={(event) =>
            setAutoImages((current) =>
              event.target.checked
                ? [...current, image.name]
                : current.filter((selected) => selected !== image.name),
            )
          }
        />
      </td>
      <td className="px-3 py-2 font-mono">{image.name}</td>
      <td className="px-3 py-2 font-mono text-xs">{image.version || "Not specified"}</td>
      <td className="px-3 py-2 font-mono text-xs">{latestImageVersionLabel(image)}</td>
      <td
        className={`px-3 py-2 text-xs ${image.error || image.latest_status === "error" ? "text-destructive" : "text-muted-foreground"}`}
      >
        {imageStatuses(image).map((status) => (
          <div key={status}>{status}</div>
        ))}
      </td>
      <td className="px-3 py-2 text-right">
        <Button
          size="sm"
          disabled={Boolean(busy) || Boolean(image.error)}
          onClick={() => void build(image.name)}
        >
          {busy === `build:${image.name}` ? `Building ${image.name}…` : `Build ${image.name}`}
        </Button>
      </td>
    </tr>
  );
}

export default function StoreImagesTable({
  images,
  ...row
}: Omit<StoreImageRowProps, "image"> & { images: StoreImage[] }) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2">Auto</th>
            <th className="px-3 py-2">Image</th>
            <th className="px-3 py-2">Source image_version</th>
            <th className="px-3 py-2">Latest image_version</th>
            <th className="px-3 py-2">Status</th>
            <th className="px-3 py-2" />
          </tr>
        </thead>
        <tbody>
          {images.map((image) => (
            <StoreImageRow key={image.name} image={image} {...row} />
          ))}
          {images.length === 0 && (
            <tr>
              <td colSpan={6} className="px-3 py-8 text-center text-muted-foreground">
                No images in this Store.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
