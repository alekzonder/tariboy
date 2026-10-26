import { useEffect, useRef } from "react";
import { useMailSocket } from "@/hooks/useMailSocket";
import { useTasksSocket } from "@/hooks/useTasksSocket";
import { useHostQuestionSnapshot, type HostSnapshotListener } from "./useHostQuestionSnapshot";

export function HostQuestionWatcher({
  hostId,
  label,
  onSnapshot,
  onRemove,
  registerRefresher,
}: {
  hostId: string;
  label: string;
  onSnapshot: HostSnapshotListener;
  onRemove: (hostId: string) => void;
  registerRefresher: (hostId: string, refresh: (() => Promise<void>) | null) => void;
}) {
  const { target, health, sequence, socketEnabled, refresh, raiseSequence } =
    useHostQuestionSnapshot({ hostId, label, onSnapshot, onRemove, registerRefresher });
  const previousSocketStatus = useRef<"connecting" | "open" | "closed">("closed");

  const socketStatus = useTasksSocket({
    // Unresolved hosts keep the socket disabled until the baseline lands.
    target: target ?? null,
    after: sequence,
    enabled: socketEnabled,
    onHint: () => void refresh(),
    onReset: (nextSequence) => {
      raiseSequence(nextSequence);
      void refresh();
    },
  });

  // A new task question is a letter: its hint refreshes the counts at once.
  useMailSocket({
    target: target ?? null,
    enabled: socketEnabled,
    onHint: (hint) => {
      if (hint.kind === "task.question") void refresh();
    },
  });

  useEffect(() => {
    const previous = previousSocketStatus.current;
    previousSocketStatus.current = socketStatus;
    if (previous === "open" && socketStatus === "closed" && health === "healthy" && socketEnabled) {
      void refresh();
    }
  }, [health, refresh, socketEnabled, socketStatus]);

  return null;
}
