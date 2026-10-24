import { Link } from "react-router-dom";
import type { ImageRow } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { builtDate, sourceLabel } from "./builtImages";
import { BuiltImageTagActions, type TagActionHandlers } from "./BuiltImageTagActions";

function TagCell({
  image,
  newest,
  basePath,
}: {
  image: ImageRow;
  newest: boolean;
  basePath: string;
}) {
  return (
    <td className="px-3 py-2">
      <div className="flex items-center gap-2">
        <Link
          className="font-mono text-primary hover:underline"
          to={`${basePath}/${encodeURIComponent(image.name)}/${encodeURIComponent(image.tag)}`}
        >
          {image.tag}
        </Link>
        {image.tag === "latest" && <Badge variant="secondary">Latest</Badge>}
        {newest && <Badge variant="secondary">Newest</Badge>}
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
  );
}

export function BuiltImageTagRow({
  image,
  newest,
  basePath,
  actions,
}: {
  image: ImageRow;
  newest: boolean;
  basePath: string;
  actions: TagActionHandlers;
}) {
  const ref = `${image.name}:${image.tag}`;
  return (
    <tr data-testid={`built-image-${ref}`} className="border-t">
      <TagCell image={image} newest={newest} basePath={basePath} />
      <td className="px-3 py-2">{image.image_version || "Version not specified"}</td>
      <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{image.digest ?? "—"}</td>
      <td className="px-3 py-2 text-xs text-muted-foreground">{builtDate(image)}</td>
      <td className="px-3 py-2 font-mono text-xs break-all">{sourceLabel(image)}</td>
      <td className="px-3 py-2">
        <BuiltImageTagActions image={image} imageRef={ref} {...actions} />
      </td>
    </tr>
  );
}
