import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { TabsList, TabsTrigger } from "@/components/ui/tabs";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { cn } from "@/lib/utils";
import type { SidebarTab } from "./sidebarPrefs";

const TABS: ReadonlyArray<{ value: SidebarTab; label: string }> = [
  { value: "servers", label: "Servers" },
  { value: "agents", label: "Agents" },
];

/**
 * The two sidebar tabs as 24px pills: the active one takes the `--accent`
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
            data-tour={tab.value === "servers" ? TOUR_ANCHORS.serversTab : undefined}
            className={cn(
              "h-6 rounded-control px-2.25 py-0 text-ui font-normal text-muted-foreground shadow-none",
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
        className="ml-auto size-6 shrink-0 rounded-control text-muted-foreground hover:bg-accent hover:text-foreground"
        aria-label="Add agent"
        title="Add agent"
        data-tour={TOUR_ANCHORS.addAgent}
        onClick={onCreate}
      >
        <Plus className="size-3" />
      </Button>
    </div>
  );
}
