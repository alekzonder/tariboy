import { useEffect, useRef, type ReactNode, type RefObject } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { withWidth } from "@/storybook/decorators";
import TasksWorkspace from "./TasksWorkspace";

// The native task tracker: the rail (All / My / Waiting / Notifications /
// Queues), a drag handle, and the toolbar over the task tree. It needs no
// router; the daemon and query decorators supply the host and the query client. There is no daemon behind Storybook, so the fetches fail and the
// rail, toolbar and empty state render.
//
// The view is internal state with no prop, so the Queues and Notifications
// stories click the real rail button from a mount effect, scoped to their own
// frame.
const Frame = ({
  children,
  frameRef,
}: {
  children: ReactNode;
  frameRef?: RefObject<HTMLDivElement | null>;
}) => (
  <div
    ref={frameRef}
    className="h-[620px] overflow-hidden rounded-md border border-border bg-background"
  >
    {children}
  </div>
);

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

const queueLabels = ["Queues", "New queue"];
const notificationLabels = ["Notifications"];

const Clicked = ({ labels }: { labels: string[] }) => {
  const frameRef = useRef<HTMLDivElement | null>(null);
  useClickSequence(frameRef, labels);
  return (
    <Frame frameRef={frameRef}>
      <TasksWorkspace />
    </Frame>
  );
};

const meta = {
  component: TasksWorkspace,
  decorators: [withWidth(1024)],
  parameters: { layout: "fullscreen", tariboy: { daemon: true, query: true } },
} satisfies Meta<typeof TasksWorkspace>;
export default meta;

type Story = StoryObj;

export const AllTasks: Story = {
  render: () => (
    <Frame>
      <TasksWorkspace />
    </Frame>
  ),
};

export const ScopedToAgent: Story = {
  render: () => (
    <Frame>
      <TasksWorkspace scopeAgent="builder" />
    </Frame>
  ),
};

export const Queues: Story = {
  render: () => <Clicked labels={queueLabels} />,
};

export const Notifications: Story = {
  render: () => <Clicked labels={notificationLabels} />,
};
