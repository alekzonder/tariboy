import { useMemo, useState } from "react";
import changelog from "../../../../CHANGELOG.md?raw";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { parseChangelog } from "@/lib/onboarding/changelog";
import { compareVersions } from "@/lib/onboarding/tour";
import { UI_VERSION } from "@/lib/onboarding/version";
import { cn } from "@/lib/utils";
import { MarkdownContent } from "@/pages/tasks/TaskMarkdown";

/**
 * CHANGELOG.md of this UI build: the versions on the left, the selected
 * version on the right. Versions after `since` (the one the user saw last)
 * carry a New mark. A file without version headings is shown whole.
 */
export default function ChangelogDialog({
  since,
  onClose,
}: {
  since: string;
  onClose: () => void;
}) {
  const entries = useMemo(() => parseChangelog(changelog), []);
  const [selected, setSelected] = useState(entries[0]?.version ?? "");
  const entry = entries.find((item) => item.version === selected);
  const isNew = (version: string) =>
    since !== "" &&
    compareVersions(version, since) > 0 &&
    compareVersions(version, UI_VERSION) <= 0;
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="flex h-[min(720px,calc(100dvh-2rem))] w-[min(960px,calc(100vw-2rem))] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-none">
        <div className="border-b px-5 py-3">
          <DialogTitle>Changelog</DialogTitle>
          <DialogDescription>Changes in each Tariboy version.</DialogDescription>
        </div>
        <div className="flex min-h-0 flex-1">
          {entries.length > 0 && (
            <nav aria-label="Versions" className="w-40 shrink-0 overflow-y-auto border-r p-2">
              {entries.map((item) => (
                <button
                  key={item.version}
                  type="button"
                  aria-current={item.version === selected ? "true" : undefined}
                  onClick={() => setSelected(item.version)}
                  className={cn(
                    "flex w-full items-center gap-1.5 rounded-control px-2 py-1 text-left text-body-sm hover:bg-accent",
                    item.version === selected && "bg-accent font-medium",
                  )}
                >
                  <span className="font-mono">{item.version}</span>
                  {isNew(item.version) && <Badge>New</Badge>}
                </button>
              ))}
            </nav>
          )}
          <div className="min-w-0 flex-1 overflow-y-auto px-5 py-4">
            {entry ? (
              <>
                <h2 className="text-title font-semibold">{entry.version}</h2>
                {entry.date && (
                  <p className="mb-3 text-caption text-muted-foreground">{entry.date}</p>
                )}
                <MarkdownContent>{entry.body}</MarkdownContent>
              </>
            ) : (
              <MarkdownContent>{changelog}</MarkdownContent>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
