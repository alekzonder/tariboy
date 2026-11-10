import { useEffect, useRef, useState } from "react";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { Label } from "@/components/ui/label";
import { PathAutocomplete } from "./PathAutocomplete";

// PathAutocomplete is controlled: the caller owns the path string (New agent →
// cwd, the Configuration tab's working directory, the terminals Create-agent
// dialog). The dropdown's folder listing comes from the daemon's /api/fs/list,
// so with no daemon the list is the "nothing to offer for this prefix" branch —
// the input itself, its placeholder and the popover chrome are all real.

const withField: Decorator = (Story) => (
  <div className="flex w-115 flex-col gap-1.5">
    <Label htmlFor="cwd">cwd</Label>
    <Story />
    {/* Room for the absolutely-positioned dropdown, which overlays the form
        row in the product rather than pushing it down. */}
    <div className="h-35" />
  </div>
);

const meta = {
  component: PathAutocomplete,
  decorators: [withField],
  parameters: { tariboy: { daemon: true } },
} satisfies Meta<typeof PathAutocomplete>;
export default meta;

type Story = StoryObj;

function Controlled({ initial, debounceMs }: { initial: string; debounceMs?: number }) {
  const [value, setValue] = useState(initial);
  return (
    <PathAutocomplete
      id="cwd"
      value={value}
      onChange={setValue}
      placeholder="empty = managed workdir"
      aria-label="cwd"
      debounceMs={debounceMs}
    />
  );
}

// The resting state in the New agent form: a typed path, list closed.
export const WorkingDirectory: Story = {
  render: () => <Controlled initial="/home/agent/github/tariboy" />,
};

// Empty means "managed workdir" — the placeholder carries that rule.
export const ManagedWorkdir: Story = {
  render: () => <Controlled initial="" />,
};

function FocusWhenMounted({ children }: { children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let tries = 0;
    const id = window.setInterval(() => {
      const input = ref.current?.querySelector("input");
      if (!input) {
        if (++tries > 40) window.clearInterval(id);
        return;
      }
      window.clearInterval(id);
      input.focus();
    }, 25);
    return () => window.clearInterval(id);
  }, []);
  return <div ref={ref}>{children}</div>;
}

// Focused, so the component's own dropdown is open. The listing request is made
// for real and fails (no daemon), which is exactly the path that renders
// "No matching folders".
export const OpenListing: Story = {
  render: () => (
    <FocusWhenMounted>
      <Controlled initial="/home/agent/github/tariboy/ui/sr" debounceMs={0} />
    </FocusWhenMounted>
  ),
};
