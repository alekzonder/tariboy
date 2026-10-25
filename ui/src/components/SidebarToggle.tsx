import { Menu, PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useSharedSidebarState } from "@/pages/terminals/sidebarStateContext";

// The titlebar button that opens the agent drawer on a compact layout, or
// shows and hides the agent sidebar otherwise.
export function SidebarToggle() {
  const sidebar = useSharedSidebarState();
  if (sidebar.compact) {
    return (
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        className="size-9 shrink-0 rounded-control text-muted-foreground hover:bg-accent hover:text-foreground"
        aria-label="Open menu"
        title="Open menu"
        aria-expanded={sidebar.drawerOpen}
        onClick={() => sidebar.setDrawerOpen(true)}
      >
        <Menu className="size-4.5" />
      </Button>
    );
  }
  const label = sidebar.hidden ? "Show agents" : "Hide agents";
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      className="size-6.5 shrink-0 rounded-control text-muted-foreground hover:bg-accent hover:text-foreground"
      aria-label={label}
      title={label}
      onClick={() => sidebar.setHidden(!sidebar.hidden)}
    >
      {sidebar.hidden ? (
        <PanelLeftOpen className="size-4" />
      ) : (
        <PanelLeftClose className="size-4" />
      )}
    </Button>
  );
}
