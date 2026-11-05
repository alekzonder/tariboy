import { useEffect, useRef, type ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { GroupWizard } from "./GroupWizard";

// GroupWizard is the multi-section group builder: section 1 is the group (name +
// base cwd + which row leads), section 2 a repeatable list of agent drafts. It
// only fetches the image catalogue (to fill the per-row ImageCombobox), so with
// no daemon the whole form still renders as shipped — the combobox is simply
// offered no options.
//
// Every state below is reached by driving the component's own controls (native
// value sets + bubbling events, real clicks); nothing is stubbed or redrawn.

type Step = { selector: string; value?: string; click?: boolean; text?: string };

function findTarget(root: HTMLElement, step: Step): HTMLElement | undefined {
  const all = [...root.querySelectorAll<HTMLElement>(step.selector)];
  const text = step.text;
  return text ? all.find((node) => node.textContent?.includes(text)) : all[0];
}

function runStep(root: HTMLElement, step: Step) {
  const el = findTarget(root, step);
  if (!el) return;
  if (step.click) {
    el.click();
  } else if (step.value !== undefined) {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    setter?.call(el, step.value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  }
}

function Drive({ steps, children }: { steps: Step[]; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const root = ref.current;
      if (root) for (const step of steps) runStep(root, step);
    }, 150);
    return () => window.clearTimeout(timer);
  }, [steps]);
  return <div ref={ref}>{children}</div>;
}

const namedSteps: Step[] = [
  { selector: "#grp-name", value: "dev-team" },
  { selector: "#name-0", value: "builder" },
];

const advancedSteps: Step[] = [
  ...namedSteps,
  { selector: '[data-slot="agent-row"] button', text: "Advanced", click: true },
];

const meta = {
  component: GroupWizard,
  parameters: { tariboy: { daemon: true } },
  decorators: [
    (Story) => (
      <div className="w-[720px]">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof GroupWizard>;
export default meta;

type Story = StoryObj<typeof meta>;

// First open: one empty draft row, Create group disabled, and the leader-name
// gate spelled out in destructive text.
export const EmptyDraft: Story = {};

// Group named and the lead row named: the `lead:` line resolves and the
// leader-name warning clears. Create stays disabled until the row has an image.
export const NamedGroup: Story = {
  render: (args) => (
    <Drive steps={namedSteps}>
      <GroupWizard {...args} />
    </Drive>
  ),
};

// The per-row Advanced panel opened through its own Collapsible trigger.
export const AdvancedOpen: Story = {
  render: (args) => (
    <Drive steps={advancedSteps}>
      <GroupWizard {...args} />
    </Drive>
  ),
};
