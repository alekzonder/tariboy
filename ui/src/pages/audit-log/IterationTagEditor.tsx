import { useState } from "react";
import { useApi } from "@/components/apiContext";
import type { IterationRow } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";

// IterationTagEditor edits the tags of the one selected iteration. Tag edits
// replace the whole set and hand the applied tags to onApplied, which patches
// the loaded row in place, so the badge list updates before the next poll.
export function IterationTagEditor({
  name,
  item,
  onApplied,
}: {
  name: string;
  item: IterationRow;
  onApplied: (id: string, tags: string[]) => void;
}) {
  const api = useApi();
  const [draftTag, setDraftTag] = useState("");
  const [tagError, setTagError] = useState("");
  const tags = item.tags ?? [];
  const saveTags = (id: string, next: string[]) => {
    setTagError("");
    void api.iterations
      .setIterationTags(name, id, next)
      .then((r) => onApplied(id, r.tags?.[id] ?? next))
      .catch((e: unknown) => setTagError(e instanceof Error ? e.message : "could not save tags"));
  };

  return (
    <div className="mb-2 flex flex-wrap items-center gap-1">
      {tags.map((tag) => (
        <Badge key={tag} variant="outline" className="gap-1">
          {tag}
          <button
            aria-label={`Remove tag ${tag}`}
            onClick={() =>
              saveTags(
                item.id,
                tags.filter((t) => t !== tag),
              )
            }
          >
            ×
          </button>
        </Badge>
      ))}
      <Input
        aria-label="Add tag"
        className="h-7 w-32"
        placeholder="add tag"
        value={draftTag}
        onChange={(e) => setDraftTag(e.target.value)}
        onKeyDown={(e) => {
          if (e.key !== "Enter" || !draftTag.trim()) return;
          saveTags(item.id, [...tags.filter((t) => t !== draftTag.trim()), draftTag.trim()]);
          setDraftTag("");
        }}
      />
      {tagError && (
        <span role="alert" className="text-xs text-destructive">
          {tagError}
        </span>
      )}
    </div>
  );
}
