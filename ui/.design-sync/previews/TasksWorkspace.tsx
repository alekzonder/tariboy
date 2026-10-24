import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { DaemonProvider, TasksWorkspace, QueryClientProvider, createQueryClient } from "tariboy-ui";

// TasksWorkspace is the native task tracker: the "Tasks" rail (All / My /
// Waiting / Notifications / Queues plus the queue picker), a drag handle, and
// the center column — toolbar with search, status filter, refresh and "New
// task" over the task tree. Selecting a task docks TaskDetail on the right.
//
// It needs no router (nothing in this subtree links) — DaemonProvider and an isolated query client,
// because the workspace remounts itself per active daemon. `target={null}` pins
// it to the same-origin local daemon instead of the registry.
//
// There is no daemon behind the capture server, so the queue, notification and
// tree fetches fail: the rail, the toolbar and the component's own empty state
// ("No tasks match this view.") are what render — the chrome this card is for.
//
// The view is internal state with no prop, so the Queues and Notifications
// stories click the real rail button from a mount effect, scoped to their own
// frame so a multi-story card never cross-fires.
const Shell = ({
  children,
  frameRef,
}: {
  children: ReactNode;
  frameRef?: RefObject<HTMLDivElement | null>;
}) => {
  const [client] = useState(createQueryClient);
  return (
    <QueryClientProvider client={client}>
      <div
        ref={frameRef}
        style={{
          height: 620,
          width: 1180,
          overflow: "hidden",
          border: "1px solid var(--border)",
          borderRadius: "var(--radius)",
          background: "var(--background)",
        }}
      >
        {children}
      </div>
    </QueryClientProvider>
  );
};

// Click real buttons by their visible label, one per frame, so each click sees
// the DOM the previous one produced.
function useClickSequence(frameRef: RefObject<HTMLDivElement | null>, labels: string[]) {
  useEffect(() => {
    let index = 0;
    let frame = 0;
    let attempts = 0;
    const step = () => {
      if (index >= labels.length || attempts++ > 90) return;
      const label = labels[index];
      const buttons = Array.from(frameRef.current?.querySelectorAll("button") ?? []);
      const match = buttons.find((button) => button.textContent?.trim() === label);
      if (match) index += 1;
      match?.click();
      frame = window.requestAnimationFrame(step);
    };
    frame = window.requestAnimationFrame(step);
    return () => window.cancelAnimationFrame(frame);
  }, [frameRef, labels]);
}

export const AllTasks = () => (
  <DaemonProvider>
    <Shell>
      <TasksWorkspace target={null} />
    </Shell>
  </DaemonProvider>
);

export const ScopedToAgent = () => (
  <DaemonProvider>
    <Shell>
      <TasksWorkspace target={null} scopeAgent="builder" />
    </Shell>
  </DaemonProvider>
);

const queueLabels = ["Queues", "New queue"];
const notificationLabels = ["Notifications"];

export const Queues = () => {
  const frameRef = useRef<HTMLDivElement | null>(null);
  useClickSequence(frameRef, queueLabels);
  return (
    <DaemonProvider>
      <Shell frameRef={frameRef}>
        <TasksWorkspace target={null} />
      </Shell>
    </DaemonProvider>
  );
};

export const Notifications = () => {
  const frameRef = useRef<HTMLDivElement | null>(null);
  useClickSequence(frameRef, notificationLabels);
  return (
    <DaemonProvider>
      <Shell frameRef={frameRef}>
        <TasksWorkspace target={null} />
      </Shell>
    </DaemonProvider>
  );
};
