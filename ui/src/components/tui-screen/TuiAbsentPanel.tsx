import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { SendFilesButton } from "@/components/SendFilesButton";
import type { useFileDropTarget } from "@/hooks/useFileDropTarget";
import { cn } from "@/lib/utils";

/** Shown instead of the terminal when the daemon reports no interactive
 * session: an optional Start button plus a file drop target and Send files. */
export function TuiAbsentPanel({
  drop,
  onStart,
  fill,
  workspaceSurface,
}: {
  drop: ReturnType<typeof useFileDropTarget>;
  onStart?: () => void;
  fill: boolean;
  workspaceSurface: boolean;
}) {
  return (
    <div
      data-testid="tui-absent-drop-target"
      data-file-drag-active={drop.dragActive}
      onDragOver={drop.onDragOver}
      onDragLeave={drop.onDragLeave}
      onDrop={drop.onDrop}
      className={cn(
        "flex flex-col items-center justify-center gap-2 bg-zinc-950 text-zinc-400",
        workspaceSurface ? "border-0" : "rounded-md border",
        fill ? "h-full" : "h-[32rem]",
        drop.dragActive && "ring-2 ring-primary",
      )}
    >
      <div>Session not running or not interactive</div>
      <div className="text-xs">
        Start the agent and enable interactive mode (Settings → Runtime config).
      </div>
      <div className="mt-2 flex items-center gap-2">
        {onStart && (
          <Button size="sm" onClick={onStart}>
            Start
          </Button>
        )}
        {/* Files still upload to the server even with no live session,
            so the operator can pre-stage them; report the saved paths. */}
        <SendFilesButton onUploaded={(paths) => toast.success(`uploaded: ${paths.join(", ")}`)} />
      </div>
    </div>
  );
}
