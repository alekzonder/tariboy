import type { TaskEvent, WaitingFor } from "./api";

// The workflow's own wait (a customer status or a pause) is answered in the
// workflow panel, never by a comment, so neither the banner nor a comment
// chip asks for one.
export function taskCommentWaits(waits: WaitingFor[], workflow: boolean): WaitingFor[] {
  return waits.filter((wait) => !(workflow && wait.requesting_principal === "system:workflow"));
}

// The workflow panel refetches when a new event arrives.
export function taskEventSequence(events: TaskEvent[], liveSequence: number | undefined): number {
  return Math.max(
    events.reduce((latest, event) => Math.max(latest, event.sequence), 0),
    liveSequence ?? 0,
  );
}
