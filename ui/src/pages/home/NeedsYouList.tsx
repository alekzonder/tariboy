import { lazy, useState } from "react";
import { ApiProvider } from "@/components/ApiProvider";
import { LazyBoundary } from "@/components/LazyBoundary";
import { Button } from "@/components/ui/button";
import { targetFor } from "@/lib/terminalsHost";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import { useNeedsYou } from "@/pages/terminals/needsYouContext";
import { NeedsYouListView, type NeedsYouListViewProps } from "./NeedsYouList.view";

// Loaded on the first expand: the card brings the Markdown renderer.
const NeedsYouCard = lazy(() => import("@/pages/tasks/NeedsYouCard"));

/**
 * The tasks and letters that wait for the user, across the workspace's hosts.
 * A task opens its drawer on its own host; a letter goes to `onOpenLetter`.
 */
export function NeedsYouList({
  rows,
  showAll,
  onOpenLetter,
}: Pick<NeedsYouListViewProps, "rows" | "showAll"> & {
  onOpenLetter: (hostId: string, letterId: string) => void;
}) {
  const { hosts, loaded, reload } = useNeedsYou();
  const [task, setTask] = useState<{ hostId: string; key: string } | null>(null);
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set());
  const toggle = (id: string) =>
    setExpanded((current) => {
      const next = new Set(current);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  const failed = loaded && hosts.length > 0 && hosts.every((host) => host.error);

  if (!loaded) return <p className="px-2 text-body-sm text-muted-foreground">Loading…</p>;
  if (failed) {
    return (
      <div role="alert" className="flex items-center gap-2 px-2 text-body-sm text-destructive">
        Could not read what needs you: {hosts[0]?.error}
        <Button variant="outline" size="sm" onClick={reload}>
          Retry
        </Button>
      </div>
    );
  }
  if (rows.length === 0) {
    return <p className="px-2 text-body-sm text-muted-foreground">Nothing needs you.</p>;
  }
  return (
    <>
      <NeedsYouListView
        rows={rows}
        expanded={expanded}
        onToggle={toggle}
        showAll={showAll}
        onOpen={(row) =>
          row.kind === "task"
            ? setTask({ hostId: row.hostId, key: row.ref })
            : onOpenLetter(row.hostId, row.ref)
        }
        renderCard={(row) => (
          <ApiProvider target={targetFor(row.hostId)}>
            <LazyBoundary resetKey={row.id}>
              <NeedsYouCard taskKey={row.ref} onChanged={reload} />
            </LazyBoundary>
          </ApiProvider>
        )}
      />
      {task && (
        <ApiProvider target={targetFor(task.hostId)}>
          <TaskDrawer
            key={`${task.hostId}\u0000${task.key}`}
            taskKey={task.key}
            onClose={() => {
              setTask(null);
              reload();
            }}
          />
        </ApiProvider>
      )}
    </>
  );
}
