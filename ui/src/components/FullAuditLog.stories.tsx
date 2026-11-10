import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, type ReactNode } from "react";
import { FullAuditLog } from "./FullAuditLog";

// FullAuditLog pages one agent's audit history: recent events first, older
// pages in on scroll-up, newer ones appended by a poll plus an SSE tail. A
// type/text filter switches the pane into a server-side query view. Storybook
// has no daemon, so every read fails and the component lands in its real empty
// state: the filter toolbar above a bordered, muted pane reading "No events."
const meta = {
  component: FullAuditLog,
  parameters: { tariboy: { daemon: true } },
} satisfies Meta<typeof FullAuditLog>;
export default meta;

type Story = StoryObj;

function useType(selector: string, value: string) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const input = ref.current?.querySelector<HTMLInputElement>(selector);
      if (!input) return;
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
      setter?.call(input, value);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    }, 120);
    return () => window.clearTimeout(timer);
  }, [selector, value]);
  return ref;
}

const Pane = ({ children }: { children: ReactNode }) => (
  <div className="h-95 w-190">{children}</div>
);

function TypedLog({ name, selector, value }: { name: string; selector: string; value: string }) {
  const ref = useType(selector, value);
  return (
    <div ref={ref}>
      <Pane>
        <FullAuditLog name={name} />
      </Pane>
    </div>
  );
}

export const EmptyLog: Story = {
  render: () => (
    <Pane>
      <FullAuditLog name="builder" />
    </Pane>
  ),
};

// Filtered mode: a filter reveals the Clear button and the match-count line, and
// pauses the paging/tail machinery. The pane shows "No matching events."
export const Filtered: Story = {
  render: () => (
    <TypedLog name="reviewer" selector='input[aria-label="Search text"]' value="TB-142" />
  ),
};

export const FilteredByType: Story = {
  render: () => (
    <TypedLog name="docs-bot" selector='input[aria-label="Filter by type"]' value="tool_use" />
  ),
};
