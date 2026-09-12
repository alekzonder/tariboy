import { useEffect, useRef } from "react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "tariboy-ui";

// ContextMenu.Root is uncontrolled by design (Radix exposes no open/defaultOpen
// for it — the point is the pointer position). To render the open state
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

const rowStyle: React.CSSProperties = {
  display: "flex",
  width: 260,
  alignItems: "center",
  gap: 6,
  padding: "6px 8px",
  borderRadius: 6,
  fontSize: 13,
  border: "1px solid var(--border)",
};

const Sidebar = ({ children }: { children?: React.ReactNode }) => (
  <div style={{ display: "grid", gap: 6, width: 260 }}>
    <div style={{ fontSize: 12, color: "var(--muted-foreground)" }}>
      This daemon (local)
    </div>
    {children}
  </div>
);

export const AgentRow = () => {
  const ref = useRightClick();
  return (
    <Sidebar>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div ref={ref} style={rowStyle}>
            <span>builder</span>
            <span style={{ marginLeft: "auto", fontSize: 12, color: "var(--muted-foreground)" }}>
              worker:v2
            </span>
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem>Clone</ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
      <div style={{ ...rowStyle, border: "1px solid transparent" }}>
        <span>reviewer</span>
      </div>
      <div style={{ ...rowStyle, border: "1px solid transparent" }}>
        <span>packager</span>
      </div>
    </Sidebar>
  );
};

export const AgentRowActions = () => {
  const ref = useRightClick();
  return (
    <Sidebar>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div ref={ref} style={rowStyle}>
            <span>packager</span>
            <span style={{ marginLeft: "auto", fontSize: 12, color: "var(--muted-foreground)" }}>
              stopped
            </span>
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem>Clone</ContextMenuItem>
          <ContextMenuItem>Rename</ContextMenuItem>
          <ContextMenuItem>Move to team</ContextMenuItem>
          <ContextMenuItem>Open working directory</ContextMenuItem>
          <ContextMenuItem className="text-destructive">Delete agent</ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
      <div style={{ ...rowStyle, border: "1px solid transparent" }}>
        <span>docs-bot</span>
      </div>
    </Sidebar>
  );
};

export const DisabledItems = () => {
  const ref = useRightClick();
  return (
    <Sidebar>
      <div style={{ fontSize: 12, color: "var(--destructive)" }}>
        build-02 is unreachable
      </div>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <div ref={ref} style={rowStyle}>
            <span>docs-bot</span>
            <span style={{ marginLeft: "auto", fontSize: 12, color: "var(--muted-foreground)" }}>
              bare:latest
            </span>
          </div>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem disabled>Clone</ContextMenuItem>
          <ContextMenuItem disabled>Rename</ContextMenuItem>
          <ContextMenuItem>Copy agent name</ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </Sidebar>
  );
};
