import { useState } from "react";
import { toast } from "sonner";
import { getStatusHistory, ApiError } from "@/lib/api";
import type { StatusHistoryEvent } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { ScrollArea } from "@/components/ui/scroll-area";

// StatusChatHistory is a button that opens a sheet listing an agent's
// status-message timeline (read from the audit log). History is lazy-fetched
// when the sheet opens, so a closed history costs nothing.
export function StatusChatHistory({ name }: { name: string }) {
  const [events, setEvents] = useState<StatusHistoryEvent[]>([]);
  const load = () =>
    getStatusHistory(name).then(
      (r) => setEvents(r.events ?? []),
      (e) => toast.error(e instanceof ApiError ? e.message : String(e)),
    );

  const items = events.map((event, index) => {
    const iterationId = event.iteration_id ?? "";
    const previousIterationId = events[index - 1]?.iteration_id ?? "";
    return {
      event,
      showSeparator: index === 0 || iterationId !== previousIterationId,
      label: iterationId ? `Iteration: ${iterationId}` : "Outside an iteration",
    };
  });

  return (
    <Sheet onOpenChange={(o) => { if (o) void load(); }}>
      <SheetTrigger asChild>
        <Button size="sm" variant="ghost" aria-label="status history">history</Button>
      </SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Status history — {name}</SheetTitle>
          <SheetDescription>Status updates, newest first, grouped by iteration.</SheetDescription>
        </SheetHeader>
        <ScrollArea className="h-[80vh] p-2">
          {items.map(({ event, showSeparator, label }, i) => {
            return (
              <div key={i}>
                {showSeparator && <h3 className="border-b py-2 text-xs font-semibold text-muted-foreground">{label}</h3>}
                <div className="border-b py-2 text-sm last:border-0">
                  <div className="text-xs text-muted-foreground">{event.ts}</div>
                  <div>{event.message}</div>
                </div>
              </div>
            );
          })}
          {events.length === 0 && <p className="text-sm text-muted-foreground">No status events.</p>}
        </ScrollArea>
      </SheetContent>
    </Sheet>
  );
}
