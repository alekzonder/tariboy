import type { ImageRow } from "@/lib/api";

function buildTime(image: ImageRow): number {
  const value = Date.parse(image.built_at ?? "");
  return Number.isFinite(value) ? value : -Infinity;
}

function compareBuiltImages(a: ImageRow, b: ImageRow): number {
  const time = buildTime(b) - buildTime(a);
  if (time && !Number.isNaN(time)) return time;
  const rank = (image: ImageRow) =>
    image.image_version && image.tag === image.image_version ? 0 : 1;
  return rank(a) - rank(b) || (a.tag < b.tag ? -1 : a.tag > b.tag ? 1 : 0);
}

export function builtDate(image?: ImageRow): string {
  return image && Number.isFinite(buildTime(image)) ? image.built_at! : "—";
}

export interface BuiltImageGroup {
  name: string;
  tags: ImageRow[];
  latest?: ImageRow;
}

export function groupBuiltImages(images: ImageRow[]): BuiltImageGroup[] {
  const byName = new Map<string, ImageRow[]>();
  for (const image of images) {
    const tags = byName.get(image.name) ?? [];
    tags.push(image);
    byName.set(image.name, tags);
  }
  return [...byName]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([name, tags]) => ({
      name,
      tags: tags.sort(compareBuiltImages),
      latest: tags.find((tag) => tag.tag === "latest"),
    }));
}

export function sourceLabel(image: ImageRow): string {
  if (!image.source_cwd) return "Source CWD unavailable — imported artifact";
  if (image.source_available === false) return `Source CWD unavailable — ${image.source_cwd}`;
  return image.source_cwd;
}

export function splitImageRef(ref: string): { name: string; tag: string } {
  const separator = ref.lastIndexOf(":");
  return { name: ref.slice(0, separator), tag: ref.slice(separator + 1) };
}

export function archiveFilename(ref: string): string {
  const { name, tag } = splitImageRef(ref);
  return `${name.replaceAll("/", "_")}-${tag}.tariboy-image.tar.gz`;
}
