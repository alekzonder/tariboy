import { useEffect, useRef } from "react";
import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "./context-menu";

const meta = { component: ContextMenu } satisfies Meta<typeof ContextMenu>;
export default meta;

type Story = StoryObj;

// ContextMenu.Root is uncontrolled by design (Radix exposes no open/defaultOpen
// for it, the point is the pointer position). To render the open state
// statically we replay the same native `contextmenu` event a right-click would
// send, at a point over the trigger. Nothing about the component is faked.
function useRightClick(dx = 90, dy = 18) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    const rect = node.getBoundingClientRect();
    node.dispatchEvent(
      new MouseEvent("contextmenu", {
        bubbles: true,
        cancelable: true,
        clientX: Math.round(rect.left + dx),
        clientY: Math.round(rect.top + dy),
      }),
    );
  }, [dx, dy]);
  return ref;
}

const rowClass = "flex w-[260px] items-center gap-1.5 rounded-md border px-2 py-1.5 text-[13px]";
const plainRow = `${rowClass} border-transparent`;
const hint = "ml-auto text-xs text-muted-foreground";

const Sidebar = ({ children }: { children?: ReactNode }) => (
  <div className="grid w-[260px] gap-1.5">
    <div className="text-xs text-muted-foreground">This daemon (local)</div>
    {children}
  </div>
);

const MenuRow = ({ name, note, children }: { name: string; note: string; children: ReactNode }) => {
  const ref = useRightClick();
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div ref={ref} className={`${rowClass} border-border`}>
          <span>{name}</span>
          <span className={hint}>{note}</span>
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>{children}</ContextMenuContent>
    </ContextMenu>
  );
};

export const AgentRow: Story = {
  render: () => (
    <Sidebar>
      <MenuRow name="builder" note="worker:v2">
        <ContextMenuItem>Clone</ContextMenuItem>
      </MenuRow>
      <div className={plainRow}>
        <span>reviewer</span>
      </div>
      <div className={plainRow}>
        <span>packager</span>
      </div>
    </Sidebar>
  ),
};

export const AgentRowActions: Story = {
  render: () => (
    <Sidebar>
      <MenuRow name="packager" note="stopped">
        <ContextMenuItem>Clone</ContextMenuItem>
        <ContextMenuItem>Rename</ContextMenuItem>
        <ContextMenuItem>Move to team</ContextMenuItem>
        <ContextMenuItem>Open working directory</ContextMenuItem>
        <ContextMenuItem className="text-destructive">Delete agent</ContextMenuItem>
      </MenuRow>
      <div className={plainRow}>
        <span>docs-bot</span>
      </div>
    </Sidebar>
  ),
};

export const DisabledItems: Story = {
  render: () => (
    <Sidebar>
      <div className="text-xs text-destructive">build-02 is unreachable</div>
      <MenuRow name="docs-bot" note="bare:latest">
        <ContextMenuItem disabled>Clone</ContextMenuItem>
        <ContextMenuItem disabled>Rename</ContextMenuItem>
        <ContextMenuItem>Copy agent name</ContextMenuItem>
      </MenuRow>
    </Sidebar>
  ),
};
