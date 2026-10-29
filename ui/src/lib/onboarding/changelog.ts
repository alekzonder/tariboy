export interface ChangelogEntry {
  version: string;
  date: string;
  body: string;
}

const HEADING = /^## \[([^\]]+)\](?: - (\S+))?\s*$/;
const LINK_DEFINITION = /^\[[^\]]+\]: \S+\s*$/;

/** Splits CHANGELOG.md at its `## [VERSION] - DATE` headings, in file order
 *  (newest first). Reference link lines are dropped: the headings that used
 *  them are not rendered. A file without such headings gives no entries. */
export function parseChangelog(markdown: string): ChangelogEntry[] {
  const entries: ChangelogEntry[] = [];
  let lines: string[] = [];
  const close = () => {
    const entry = entries.at(-1);
    if (entry) entry.body = lines.join("\n").trim();
    lines = [];
  };
  for (const line of markdown.split("\n")) {
    const heading = HEADING.exec(line);
    if (heading) {
      close();
      entries.push({ version: heading[1], date: heading[2] ?? "", body: "" });
    } else if (!LINK_DEFINITION.test(line)) {
      lines.push(line);
    }
  }
  close();
  return entries;
}
