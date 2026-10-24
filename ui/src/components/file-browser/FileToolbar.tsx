import { FilePlus, FolderPlus } from "lucide-react";
import { Button } from "@/components/ui/button";

export function FileToolbar({
  canCreate,
  onNew,
}: {
  canCreate: boolean;
  onNew: (kind: "file" | "dir") => void;
}) {
  return (
    <div className="flex shrink-0 items-center gap-1 border-b px-2 py-1">
      <span className="mr-auto text-xs font-medium text-muted-foreground">Files</span>
      {canCreate && (
        <>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="New file"
            title="New file"
            onClick={() => onNew("file")}
          >
            <FilePlus className="size-3.5" />
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="New folder"
            title="New folder"
            onClick={() => onNew("dir")}
          >
            <FolderPlus className="size-3.5" />
          </Button>
        </>
      )}
    </div>
  );
}
