import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";
import type { SidebarTab } from "./sidebarPrefs";

const TABS: ReadonlyArray<{ value: SidebarTab; label: string }> = [
  { value: "agents", label: "Agents" },
  { value: "groups", label: "Groups" },
  { value: "servers", label: "Servers" },
];

/**
 * The three sidebar tabs as 24px pills: the active one takes the `--accent`
 * fill, the rest are bare muted text. The trailing `+` creates an agent on the
 * server the operator is looking at.
 */
export function SidebarTabs({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="flex shrink-0 items-center gap-1.5 px-2 pb-2">
      <TabsList className="h-auto w-auto gap-1.5 rounded-none bg-transparent p-0">
        {TABS.map((tab) => (
          <TabsTrigger
            key={tab.value}
            value={tab.value}
            className={cn(
              "h-6 rounded-[7px] px-[9px] py-0 text-[12px] font-normal text-muted-foreground shadow-none",
              "data-[state=active]:bg-accent data-[state=active]:font-medium data-[state=active]:text-foreground data-[state=active]:shadow-none",
            )}
          >
            {tab.label}
          </TabsTrigger>
        ))}
      </TabsList>
      <Button
        variant="ghost"
        size="icon-xs"
        className="ml-auto size-6 shrink-0 rounded-[7px] text-muted-foreground hover:bg-accent hover:text-foreground"
        aria-label="Add agent"
        title="Add agent"
        onClick={onCreate}
      >
        <Plus className="size-3" />
      </Button>
    </div>
  );
}
