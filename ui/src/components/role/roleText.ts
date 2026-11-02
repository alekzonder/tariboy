import type { ManifestRole } from "@/lib/api";

/** The ROLE.md text of an image or workflow image manifest. A missing file and a
 *  file of whitespace only are both no ROLE.md. */
export function roleText(manifest: { role?: ManifestRole } | null | undefined): string | null {
  const text = manifest?.role?.text;
  return text && text.trim() ? text : null;
}

type BlockKind = "paragraph" | "heading" | "list" | "skip";
interface Block {
  kind: BlockKind;
  lines: string[];
}

const fence = /^\s*(```|~~~)/;
const heading = /^\s{0,3}#{1,6}(\s|$)/;
const setext = /^\s{0,3}(=+|-+)\s*$/;
const listItem = /^\s*([-*+]|\d+[.)])\s+/;
const rule = /^\s{0,3}([-*_])(\s*\1){2,}\s*$/;

function blockKind(lines: string[]): BlockKind {
  const [first] = lines;
  if (heading.test(first) || (lines.length === 2 && setext.test(lines[1]))) return "heading";
  if (listItem.test(first)) return "list";
  if (rule.test(first) || first.trim().startsWith("|")) return "skip";
  return "paragraph";
}

/** Blank-line separated blocks; a fenced code block is one skipped block. */
function blocks(text: string): Block[] {
  const result: Block[] = [];
  let lines: string[] = [];
  let inFence = "";
  const flush = (kind?: BlockKind) => {
    if (lines.length) result.push({ kind: kind ?? blockKind(lines), lines });
    lines = [];
  };
  for (const line of text.split(/\r?\n/)) {
    const marker = fence.exec(line)?.[1];
    if (inFence) {
      lines.push(line);
      if (marker === inFence) {
        flush("skip");
        inFence = "";
      }
    } else if (marker) {
      flush();
      inFence = marker;
      lines.push(line);
    } else if (line.trim() === "") flush();
    else lines.push(line);
  }
  flush(inFence ? "skip" : undefined);
  return result;
}

/** Plain text of one line of inline Markdown. */
function plain(markdown: string): string {
  return markdown
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/<(https?:[^>\s]+)>/g, "$1")
    .replace(/<\/?[a-z][^>]*>/gi, "")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/(\*\*|__)(.+?)\1/g, "$2")
    .replace(/~~(.+?)~~/g, "$1")
    .replace(/(^|[^\w*])[*_]([^*_\s][^*_]*?)[*_](?![\w*])/g, "$1$2")
    .replace(/\s+/g, " ")
    .trim();
}

function blockText(block: Block): string {
  if (block.kind === "list") return plain(block.lines[0].replace(listItem, ""));
  const lines =
    block.kind === "heading" && block.lines.length === 2 ? [block.lines[0]] : block.lines;
  return plain(lines.map((line) => line.replace(heading, "").replace(/^\s*>\s?/, "")).join(" "));
}

/**
 * The New agent form's short form of a ROLE.md: the first paragraph that is not
 * a heading, else the first list item, as plain text. `more` says the file has
 * other blocks, so the full text shows more than the fragment.
 */
export function roleFragment(text: string): { text: string; more: boolean } {
  const all = blocks(text);
  const chosen =
    all.find((block) => block.kind === "paragraph") ??
    all.find((block) => block.kind === "list") ??
    all.find((block) => block.kind !== "skip") ??
    all[0];
  const listMore = chosen?.kind === "list" && chosen.lines.length > 1;
  return { text: chosen ? blockText(chosen) : "", more: all.length > 1 || listMore };
}
