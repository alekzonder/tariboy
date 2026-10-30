import { Link } from "react-router-dom";
import type { ImageSummary } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { builtDate, sourceLabel } from "./builtImages";
import { BuiltImageTagActions, type TagActionHandlers } from "./BuiltImageTagActions";

function TagCell({ image, newest }: { image: ImageSummary; newest: boolean }) {
  return (
    <td className="px-3 py-2">
      <div className="flex items-center gap-2">
        <Link
          className="font-mono text-primary hover:underline"
          to={`/library/images/${encodeURIComponent(image.name)}/${encodeURIComponent(image.tag)}`}
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
  current,
  actions,
}: {
  image: ImageSummary;
  newest: boolean;
  /** The tag the page shows. */
  current: boolean;
  actions: TagActionHandlers;
}) {
  const ref = `${image.name}:${image.tag}`;
  const source = sourceLabel(image);
  return (
    <tr
      data-testid={`built-image-${ref}`}
      aria-current={current ? "true" : undefined}
      className={current ? "border-t bg-muted/40" : "border-t"}
    >
      <TagCell image={image} newest={newest} />
      <td className="px-3 py-2">{image.image_version || "Version not specified"}</td>
      <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{image.digest ?? "—"}</td>
      <td className="px-3 py-2 text-xs whitespace-nowrap text-muted-foreground">
        {builtDate(image)}
      </td>
      <td className="px-3 py-2 font-mono text-xs">
        <span className="block max-w-64 truncate" title={source}>
          {source}
        </span>
      </td>
      <td className="px-3 py-2">
        <BuiltImageTagActions image={image} imageRef={ref} {...actions} />
      </td>
    </tr>
  );
}
