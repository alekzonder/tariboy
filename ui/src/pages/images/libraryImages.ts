import type { ImageSummary } from "@/lib/api";
import { groupBuiltImages } from "./built/builtImages";

/** One image name on the Library's Images tab. */
export interface LibraryImageRow {
  name: string;
  /** The newest tag. */
  tag: string;
  version: string;
  /** The Store the newest tag with a known source came from. */
  source: string;
  builtAt: string;
  /** Agents whose current or pending image is any tag of this name. */
  usedBy: number;
  /** The ref "Run agent" starts: `latest` when it exists, else the newest tag. */
  runRef: string;
}

/** One row per image name, without `bare`: the New agent dialog reaches it
 *  as "Plain terminal". */
export function libraryImageRows(images: ImageSummary[]): LibraryImageRow[] {
  return groupBuiltImages(images)
    .filter((group) => group.name !== "bare")
    .map(({ name, tags, latest }) => {
      const newest = tags[0];
      const agents = new Set(
        tags.flatMap((image) => [...(image.current_agents ?? []), ...(image.pending_agents ?? [])]),
      );
      return {
        name,
        tag: newest.tag,
        version: newest.image_version ?? "",
        source: tags.find((image) => image.source)?.source ?? "",
        builtAt: newest.built_at,
        usedBy: agents.size,
        runRef: `${name}:${(latest ?? newest).tag}`,
      };
    });
}

/** The rows the list shows: matching the search, without unused images
 *  unless asked, the most used first and then by name. */
export function visibleImageRows(
  rows: LibraryImageRow[],
  { query, showUnused }: { query: string; showUnused: boolean },
): LibraryImageRow[] {
  const needle = query.trim().toLowerCase();
  return rows
    .filter((row) => (showUnused || row.usedBy > 0) && row.name.toLowerCase().includes(needle))
    .sort((a, b) => b.usedBy - a.usedBy || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
}

/** How many images no agent uses. */
export function unusedCount(rows: LibraryImageRow[]): number {
  return rows.filter((row) => row.usedBy === 0).length;
}
