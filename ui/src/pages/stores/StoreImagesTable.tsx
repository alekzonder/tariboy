import { Link } from "react-router-dom";
import type { StoreImageView } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { catalogRow, type CatalogRow } from "./storeStatus";
import type { ImageBuildError } from "./useStoreActions";

interface StoreImageRowProps {
  image: StoreImageView;
  busy: string;
  build: (imageName: string) => Promise<void>;
  buildErrors: Record<string, ImageBuildError>;
  operationHref: (id: number) => string;
}

const TONES: Record<CatalogRow["tone"], string> = {
  muted: "text-muted-foreground",
  update: "text-amber-700 dark:text-amber-400",
  error: "text-destructive",
};

function StatusCell({ row, href }: { row: CatalogRow; href?: string }) {
  return (
    <td className={`px-3 py-2 text-xs ${TONES[row.tone]}`}>
      <span>{row.status}</span>
      {href && (
        <>
          {" · "}
          <Link className="underline underline-offset-2" to={href}>
            Open operation
          </Link>
        </>
      )}
      {row.detail && (
        <details className="mt-1">
          <summary className="cursor-pointer">Details</summary>
          <p className="mt-1 font-mono break-all">{row.detail}</p>
        </details>
      )}
    </td>
  );
}

function StoreImageRow({ image, busy, build, buildErrors, operationHref }: StoreImageRowProps) {
  const row = catalogRow(image, buildErrors[image.name]);
  const building = busy === `build:${image.name}`;
  const verb = row.action === "update" ? "Update" : "Build";
  return (
    <tr className="border-t">
      <td className="px-3 py-2 font-mono">{image.name}</td>
      <td className="px-3 py-2 font-mono text-xs">{image.version || "—"}</td>
      <StatusCell
        row={row}
        href={row.operation === undefined ? undefined : operationHref(row.operation)}
      />
      <td className="px-3 py-2 text-right">
        {row.action && (
          <Button
            size="xs"
            variant={
              row.action === "build" && image.latest_status === "built" ? "outline" : "default"
            }
            aria-label={building ? `Building ${image.name}…` : `${verb} ${image.name}`}
            disabled={Boolean(busy)}
            onClick={() => void build(image.name)}
          >
            {building ? "Building…" : verb}
          </Button>
        )}
      </td>
    </tr>
  );
}

/** A Store's images: name, source version, one status sentence and one button. */
export default function StoreImagesTable({
  images,
  ...row
}: Omit<StoreImageRowProps, "image"> & { images: StoreImageView[] }) {
  return (
    <table className="w-full text-sm">
      <thead className="text-left text-xs text-muted-foreground">
        <tr>
          <th className="px-3 py-2 font-medium">Image</th>
          <th className="px-3 py-2 font-medium">Version</th>
          <th className="px-3 py-2 font-medium">Status</th>
          <th className="px-3 py-2" />
        </tr>
      </thead>
      <tbody>
        {images.map((image) => (
          <StoreImageRow key={image.name} image={image} {...row} />
        ))}
        {images.length === 0 && (
          <tr>
            <td colSpan={4} className="px-3 py-6 text-center text-muted-foreground">
              No images in this Store.
            </td>
          </tr>
        )}
      </tbody>
    </table>
  );
}
