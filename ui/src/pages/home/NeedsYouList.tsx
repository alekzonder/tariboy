import { useState } from "react";
import { Mail } from "lucide-react";
import { ApiProvider } from "@/components/ApiProvider";
import { Button } from "@/components/ui/button";
import type { NeedsYouRow } from "@/lib/needsYou";
import { targetFor } from "@/lib/terminalsHost";
import { principalName } from "@/pages/mailbox/mailboxModel";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import { formatTaskTime } from "@/pages/tasks/taskTime";
import { useNeedsYou } from "@/pages/terminals/needsYouContext";

const ROW_CLASS =
  "flex w-full min-w-0 items-baseline gap-3 rounded-control px-2 py-1.5 text-left text-body-sm hover:bg-accent";

function taskState(row: Extract<NeedsYouRow, { kind: "task" }>): string {
  const { task } = row;
  const waiting = task.waiting_on === "pause" ? "paused" : task.status;
  return [waiting, task.assignee].filter(Boolean).join(" · ");
}

/**
 * The tasks and letters that wait for the user, across the workspace's hosts.
 * A task opens its drawer on its own host; a letter goes to `onOpenLetter`.
 */
export function NeedsYouList({
  rows,
  multiHost,
  onOpenLetter,
}: {
  rows: NeedsYouRow[];
  multiHost: boolean;
  onOpenLetter: (hostId: string, letterId: string) => void;
}) {
  const { hosts, loaded, reload } = useNeedsYou();
  const [task, setTask] = useState<{ hostId: string; key: string } | null>(null);
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
      <ul className="space-y-0.5">
        {rows.map((row) => (
          <li key={`${row.hostId}\u0000${row.kind}\u0000${row.id}`}>
            <button
              type="button"
              className={ROW_CLASS}
              onClick={() =>
                row.kind === "task"
                  ? setTask({ hostId: row.hostId, key: row.task.key })
                  : onOpenLetter(row.hostId, row.letter.id)
              }
            >
              {row.kind === "task" ? (
                <span className="w-24 shrink-0 truncate font-mono text-caption">
                  {row.task.key}
                </span>
              ) : (
                <span className="flex w-24 shrink-0 items-center gap-1 text-caption text-muted-foreground">
                  <Mail className="size-3" aria-hidden="true" />
                  {principalName(row.letter.from)}
                </span>
              )}
              <span className="min-w-0 flex-1 truncate">
                {row.kind === "task" ? row.task.title : row.letter.subject || "(no subject)"}
              </span>
              <span className="shrink-0 truncate text-caption text-muted-foreground">
                {row.kind === "task" ? taskState(row) : "letter"}
                {multiHost ? ` · ${row.hostLabel}` : ""}
                {` · ${formatTaskTime(row.at)}`}
              </span>
            </button>
          </li>
        ))}
      </ul>
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
