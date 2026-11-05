import { useEffect, useRef } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { imageSummary } from "@/fixtures/images";
import { Label } from "@/components/ui/label";
import { ImageCombobox } from "./AgentFormFields";

// ImageCombobox is the required image picker on the agent-create surfaces
// (CreateAgentDialog and each row of the group wizard). The value
// is the full `name:tag` ref; the option list is a plain filtered listbox that
// opens on focus.

const images = [
  imageSummary({
    name: "worker",
    tag: "v2",
    built_at: "2026-09-11T09:14:00Z",
    current_agents: ["builder", "reviewer"],
    pending_agents: [],
  }),
  imageSummary({ name: "worker", tag: "v1", built_at: "2026-08-28T17:02:00Z", pending_agents: [] }),
  imageSummary({
    name: "bare",
    tag: "latest",
    bare: true,
    built_at: "2026-09-02T11:40:00Z",
    pending_agents: [],
  }),
  imageSummary({
    name: "packager",
    tag: "v3",
    built_at: "2026-09-10T20:31:00Z",
    pending_agents: [],
  }),
];

const onChange = () => {};

const meta = {
  component: ImageCombobox,
  decorators: [
    (Story) => (
      <div className="flex w-80 flex-col gap-1.5">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ImageCombobox>;
export default meta;

type Story = StoryObj;

export const Selected: Story = {
  render: () => (
    <>
      <Label htmlFor="image-selected">image *</Label>
      <ImageCombobox images={images} onChange={onChange} id="image-selected" value="worker:v2" />
    </>
  ),
};

export const EmptyRequired: Story = {
  render: () => (
    <>
      <Label htmlFor="image-empty">image *</Label>
      <ImageCombobox images={images} onChange={onChange} id="image-empty" value="" />
    </>
  ),
};

export const Invalid: Story = {
  render: () => (
    <>
      <Label htmlFor="image-invalid">image *</Label>
      <ImageCombobox images={images} onChange={onChange} id="image-invalid" value="" invalid />
      <p className="m-0 text-xs text-destructive">image is required</p>
    </>
  ),
};

function FocusOnMount({ children }: { children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.querySelector("input")?.focus({ preventScroll: true });
  }, []);
  return (
    <div ref={ref} className="flex flex-col gap-1.5 pb-[170px]">
      {children}
    </div>
  );
}

// The listbox opens on focus, so this story focuses the real input on mount
// to show the filtered option list the picker ships with.
export const OpenList: Story = {
  render: () => (
    <FocusOnMount>
      <Label htmlFor="image-open">image *</Label>
      <ImageCombobox
        images={images}
        onChange={onChange}
        id="image-open"
        value="worker:v2"
        ariaLabel="image for builder"
      />
    </FocusOnMount>
  ),
};

// The group wizard gives each row's picker a distinct accessible name and packs
// it beside the agent name, so the field lives in a tight two-column grid.
export const InGroupWizardRow: Story = {
  decorators: [
    (Story) => (
      <div className="grid w-[460px] grid-cols-2 items-end gap-3">
        <Story />
      </div>
    ),
  ],
  render: () => (
    <>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="wiz-name">name</Label>
        <div className="flex h-8 items-center rounded-md border border-input px-3 font-mono text-[13px]">
          packager
        </div>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="wiz-image">image</Label>
        <ImageCombobox
          images={images}
          onChange={onChange}
          id="wiz-image"
          value="packager:v3"
          ariaLabel="image for packager"
        />
      </div>
    </>
  ),
};
