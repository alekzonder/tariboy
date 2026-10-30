import type { ImageSummary } from "@/lib/api";
import { BuiltImageTagRow } from "./BuiltImageTagRow";
import type { TagActionHandlers } from "./BuiltImageTagActions";

export function BuiltImageTagsTable({
  tags,
  newestTag,
  currentTag,
  actions,
}: {
  tags: ImageSummary[];
  newestTag?: string;
  /** The tag the page shows. */
  currentTag?: string;
  actions: TagActionHandlers;
}) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2">Tag</th>
            <th className="px-3 py-2">Version</th>
            <th className="px-3 py-2">Digest</th>
            <th className="px-3 py-2">Built</th>
            <th className="px-3 py-2">Source</th>
            <th className="px-3 py-2" />
          </tr>
        </thead>
        <tbody>
          {tags.map((image) => (
            <BuiltImageTagRow
              key={`${image.name}:${image.tag}`}
              image={image}
              newest={image.tag === newestTag}
              current={image.tag === currentTag}
              actions={actions}
            />
          ))}
          {tags.length === 0 && (
            <tr>
              <td colSpan={6} className="px-3 py-8 text-center text-muted-foreground">
                No tags for this image on this host.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
