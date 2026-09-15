import { Plus, Server } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** The 44px foot of the sidebar: what is connected, and how to add more. */
export function SidebarFooter({ servers, connected, agents, onAddServer }: {
  servers: number;
  connected: number;
  agents: number;
  onAddServer: () => void;
}) {
  const count = (n: number, noun: string) => `${n} ${noun}${n === 1 ? "" : "s"}`;
  const healthy = connected === servers;
  const health = `${connected} of ${servers} servers connected`;
  return (
    <div className="flex h-11 shrink-0 items-center gap-2 px-3">
      <span aria-hidden="true" className="grid size-[22px] shrink-0 place-items-center rounded-full bg-accent text-muted-foreground">
        <Server className="size-3" />
      </span>
      <span className="min-w-0 flex-1 truncate text-[12px] text-muted-foreground">
        {count(servers, "server")} · {count(agents, "agent")}
      </span>
      <span
        role="img"
        aria-label={health}
        title={health}
        className={cn(
          "size-1.5 shrink-0 rounded-full",
          healthy ? "bg-status-running" : "bg-status-failed",
        )}
      />
      <Button
        variant="ghost"
        size="icon-xs"
        className="size-[22px] shrink-0 rounded-[6px] text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
        aria-label="Add host"
        title="Add host"
        onClick={onAddServer}
      >
        <Plus className="size-3" />
      </Button>
    </div>
  );
}
