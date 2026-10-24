/** The glyph toolbar: mono marks rather than icons, because each one is the
 *  Markdown it inserts. `wrap` is what goes on each side of the selection;
 *  `prefix` is for the line-leading marks. */
export const MARKS = [
  { label: "B", title: "Bold", wrap: "**" },
  { label: "i", title: "Italic", wrap: "*" },
  { label: "</>", title: "Inline code", wrap: "`" },
  { label: "“", title: "Quote", prefix: "> " },
  { label: "•", title: "Bullet list", prefix: "- " },
  { label: "↗", title: "Link", wrap: "", link: true },
] as const;

export type ComposerMark = (typeof MARKS)[number];

/** The draft with one mark applied to the selection, and where the caret goes. */
export function applyMark({
  value,
  start,
  end,
  mark,
}: {
  value: string;
  start: number;
  end: number;
  mark: ComposerMark;
}): { next: string; caret: number } {
  const selected = value.slice(start, end);
  if ("prefix" in mark && mark.prefix) {
    const lineStart = value.lastIndexOf("\n", start - 1) + 1;
    return {
      next: `${value.slice(0, lineStart)}${mark.prefix}${value.slice(lineStart)}`,
      caret: end + mark.prefix.length,
    };
  }
  if ("link" in mark && mark.link) {
    return {
      next: `${value.slice(0, start)}[${selected || "text"}](url)${value.slice(end)}`,
      caret: start + (selected || "text").length + 3,
    };
  }
  return {
    next: `${value.slice(0, start)}${mark.wrap}${selected}${mark.wrap}${value.slice(end)}`,
    caret: end + mark.wrap.length * 2,
  };
}
