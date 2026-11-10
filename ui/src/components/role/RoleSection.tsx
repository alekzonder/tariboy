import { useId, useLayoutEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import RoleMarkdown from "./RoleMarkdown";

/** A taller text folds to the first 240px (`max-h-60`). */
const FOLD_ABOVE_PX = 320;

function useTallerThan(limit: number) {
  const ref = useRef<HTMLDivElement>(null);
  const [tall, setTall] = useState(false);
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () => setTall(element.scrollHeight > limit);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [limit]);
  return { ref, tall };
}

/** The ROLE.md text, folded behind Show all when it is taller than 320px. The
 *  open state is not kept: every visit starts folded. */
function CollapsibleRole({
  text,
  headingId,
  fadeClass,
}: {
  text: string;
  headingId: string;
  fadeClass: string;
}) {
  const bodyId = useId();
  const { ref, tall } = useTallerThan(FOLD_ABOVE_PX);
  const [expanded, setExpanded] = useState(false);
  const folded = tall && !expanded;

  const showLess = () => {
    const heading = document.getElementById(headingId);
    const above = (heading?.getBoundingClientRect().top ?? 0) < 0;
    flushSync(() => setExpanded(false));
    if (above) heading?.scrollIntoView({ block: "start" });
  };

  return (
    <div>
      <div className="relative">
        <div
          ref={ref}
          id={bodyId}
          className={folded ? "max-h-60 overflow-hidden" : undefined}
          // Focus inside the folded part unfolds it, so the focused element is seen.
          onFocus={() => folded && setExpanded(true)}
        >
          <RoleMarkdown text={text} />
        </div>
        {folded && (
          <div
            aria-hidden="true"
            className={`pointer-events-none absolute inset-x-0 bottom-0 h-12 bg-linear-to-t to-transparent ${fadeClass}`}
          />
        )}
      </div>
      {tall && (
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls={bodyId}
          onClick={expanded ? showLess : () => setExpanded(true)}
          className="mt-1 h-8 rounded-control text-body text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          {expanded ? "Show less" : "Show all"}
        </button>
      )}
    </div>
  );
}

function RoleBody({
  text,
  empty,
  headingId,
  fadeClass,
}: {
  text: string | null;
  empty: string;
  headingId: string;
  fadeClass: string;
}) {
  if (text === null) return <p className="text-body text-muted-foreground">{empty}</p>;
  return <CollapsibleRole text={text} headingId={headingId} fadeClass={fadeClass} />;
}

const source = <span className="text-ui text-muted-foreground">From ROLE.md</span>;

/**
 * The Role of an image (`card`, a card on the Overview tab) or of a workflow
 * image (`section`, a section of its page). `text` is null without a ROLE.md:
 * the section then shows the `empty` line.
 */
export function RoleSection({
  variant,
  text,
  empty,
}: {
  variant: "card" | "section";
  text: string | null;
  empty: string;
}) {
  const headingId = useId();
  if (variant === "card")
    return (
      // min-w-0: a wide table scrolls inside the card, not widens the grid.
      <section aria-labelledby={headingId} className="min-w-0">
        <Card>
          <CardHeader className="flex items-baseline justify-between gap-2 pb-2">
            <h2 id={headingId} className="font-heading text-title leading-snug font-medium">
              Role
            </h2>
            {text !== null && source}
          </CardHeader>
          <CardContent>
            <RoleBody text={text} empty={empty} headingId={headingId} fadeClass="from-card" />
          </CardContent>
        </Card>
      </section>
    );
  return (
    <section aria-labelledby={headingId} className="space-y-2">
      <div className="flex items-baseline justify-between gap-2">
        <h2 id={headingId} className="font-medium">
          Role
        </h2>
        {text !== null && source}
      </div>
      {text === null ? (
        <RoleBody text={text} empty={empty} headingId={headingId} fadeClass="" />
      ) : (
        <div className="rounded-chip border p-3">
          <RoleBody text={text} empty={empty} headingId={headingId} fadeClass="from-background" />
        </div>
      )}
    </section>
  );
}
