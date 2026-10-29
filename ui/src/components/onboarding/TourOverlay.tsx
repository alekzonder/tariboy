import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cardPosition, type TourStep } from "@/lib/onboarding/tour";
import { useTourTarget } from "./useTourTarget";

interface TourOverlayProps {
  steps: readonly TourStep[];
  /** What's new: the steps carry a New or Updated mark and a changelog link. */
  whatsNew: boolean;
  onClose: () => void;
  onChangelog: () => void;
}

// The padding between an element and its highlight ring.
const RING = 4;

/**
 * The tour: the screen dims except a window around the step's element, and a
 * card next to it explains the element. Clicks on the dimmed interface do not
 * pass; the card's buttons and ←, → and Esc move or close the tour.
 */
export default function TourOverlay({ steps, whatsNew, onClose, onChangelog }: TourOverlayProps) {
  const [position, setPosition] = useState({ index: 0, direction: 1 });
  const step = steps[position.index];
  const last = position.index === steps.length - 1;
  const go = useCallback(
    (direction: number) =>
      setPosition((current) => {
        const index = current.index + direction;
        return index < 0 || index >= steps.length ? current : { index, direction };
      }),
    [steps.length],
  );
  // A step that cannot show (an agent step without agents) passes on the way it was entered.
  const skip = useCallback(() => go(position.direction), [go, position.direction]);
  const view = useTourTarget(step, skip);
  const forward = useCallback(() => (last ? onClose() : go(1)), [go, last, onClose]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "ArrowRight") forward();
      else if (event.key === "ArrowLeft") go(-1);
      else if (event.key === "Escape") onClose();
      else return;
      event.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [forward, go, onClose]);

  const rect = view.rect;
  return (
    // Above menus (z-50) and dialogs (z-60): nothing else is usable during the tour.
    <div className="fixed inset-0 z-70" data-testid="tour-overlay">
      {rect ? (
        <div
          aria-hidden="true"
          data-testid="tour-highlight"
          data-anchor={step.anchor}
          className="pointer-events-none absolute rounded-md shadow-[0_0_0_9999px_rgb(0_0_0/0.55)] ring-2 ring-primary transition-all duration-200"
          style={{
            top: rect.top - RING,
            left: rect.left - RING,
            width: rect.width + 2 * RING,
            height: rect.height + 2 * RING,
          }}
        />
      ) : (
        <div aria-hidden="true" className="absolute inset-0 bg-black/55" />
      )}
      <TourCard
        step={step}
        index={position.index}
        total={steps.length}
        whatsNew={whatsNew}
        view={view}
        onBack={() => go(-1)}
        onNext={forward}
        onClose={onClose}
        onChangelog={onChangelog}
      />
    </div>
  );
}

function TourCard({
  step,
  index,
  total,
  whatsNew,
  view,
  onBack,
  onNext,
  onClose,
  onChangelog,
}: {
  step: TourStep;
  index: number;
  total: number;
  whatsNew: boolean;
  view: ReturnType<typeof useTourTarget>;
  onBack: () => void;
  onNext: () => void;
  onClose: () => void;
  onChangelog: () => void;
}) {
  const titleId = useId();
  const bodyId = useId();
  const card = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 320, height: 180 });
  // The card's text changes per step, so its height is measured per step.
  useLayoutEffect(() => {
    const element = card.current;
    if (!element?.offsetWidth) return;
    const next = { width: element.offsetWidth, height: element.offsetHeight };
    setSize((current) =>
      current.width === next.width && current.height === next.height ? current : next,
    );
  }, [index]);
  useEffect(() => card.current?.focus(), [index]);
  const place = cardPosition(view.rect, view, size);
  const mark = whatsNew && step.since ? (step.updated ? "Updated" : "New") : null;
  return (
    <div
      ref={card}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={bodyId}
      tabIndex={-1}
      className="absolute w-80 max-w-[calc(100vw-1.5rem)] rounded-card border bg-popover p-4 text-popover-foreground shadow-lg outline-none transition-all duration-200"
      style={{ top: place.top, left: place.left }}
    >
      <div className="mb-1 flex items-center gap-2 text-caption text-muted-foreground">
        <span>
          {index + 1} of {total}
        </span>
        {mark && <Badge>{mark}</Badge>}
      </div>
      <h2 id={titleId} className="text-body-lg font-semibold">
        {step.title}
      </h2>
      <p id={bodyId} aria-live="polite" className="mt-1 text-body text-muted-foreground">
        {step.body}
      </p>
      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="ghost" size="sm" onClick={onClose}>
          Skip tour
        </Button>
        {whatsNew && (
          <Button variant="link" size="sm" className="px-0" onClick={onChangelog}>
            Full changelog
          </Button>
        )}
        <div className="ml-auto flex gap-2">
          <Button variant="outline" size="sm" disabled={index === 0} onClick={onBack}>
            Back
          </Button>
          <Button size="sm" onClick={onNext}>
            {index === total - 1 ? "Done" : "Next"}
          </Button>
        </div>
      </div>
    </div>
  );
}
