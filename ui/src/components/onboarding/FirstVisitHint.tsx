import { useState } from "react";
import { Lightbulb, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { HINT_TEXT, type HintTab } from "@/lib/onboarding/hints";
import { readKey, storageKeyFamilies, writeKey } from "@/lib/storage";
import { cn } from "@/lib/utils";

/**
 * A one-line hint on an agent tab, shown until the user dismisses it, once
 * per agent. A blocked storage hides it for this visit only, so it shows
 * again after a reload.
 */
export function FirstVisitHint({
  hostId,
  agent,
  tab,
  className,
}: {
  hostId: string;
  agent: string;
  tab: HintTab;
  className?: string;
}) {
  const key = storageKeyFamilies.onboardingHint.key(hostId, agent, tab);
  // Keyed by the storage name, so switching agents re-reads the record.
  const [dismissed, setDismissed] = useState<string | null>(null);
  if (dismissed === key.name || readKey(key)) return null;
  return (
    <div
      role="note"
      className={cn(
        "flex shrink-0 items-center gap-2 rounded-md border bg-muted/40 px-3 py-1.5 text-sm text-muted-foreground",
        className,
      )}
    >
      <Lightbulb aria-hidden="true" className="size-4 shrink-0" />
      <p className="min-w-0 flex-1">{HINT_TEXT[tab]}</p>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="Dismiss hint"
        title="Dismiss"
        onClick={() => {
          writeKey(key, true);
          setDismissed(key.name);
        }}
      >
        <X className="size-3.5" />
      </Button>
    </div>
  );
}
