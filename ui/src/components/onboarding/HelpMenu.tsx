import { CircleHelp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { useOnboarding } from "./onboardingContext";

/** The same three actions as the titlebar menu, on Application settings. */
export function HelpCard() {
  const onboarding = useOnboarding();
  if (!onboarding) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Help</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={() => onboarding.startTour("full")}>
          Take the tour
        </Button>
        <Button variant="outline" onClick={() => onboarding.startTour("whats-new")}>
          What&apos;s new
        </Button>
        <Button variant="outline" onClick={onboarding.openChangelog}>
          Changelog
        </Button>
      </CardContent>
    </Card>
  );
}

/** The titlebar "?" menu: the tour, What's new and the changelog. */
export function HelpMenu() {
  const onboarding = useOnboarding();
  if (!onboarding) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Help"
          title="Help"
          data-tour={TOUR_ANCHORS.helpMenu}
          className="size-6.5 rounded-control text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <CircleHelp className="size-3.75" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => onboarding.startTour("full")}>
          Take the tour
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onboarding.startTour("whats-new")}>
          What&apos;s new
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onboarding.openChangelog}>Changelog</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
