import { lazy, useId, useLayoutEffect, useRef, useState } from "react";
import { LazyBoundary } from "@/components/LazyBoundary";
import { roleFragment } from "./roleText";

// react-markdown loads when the reader opens the full role, never with the form.
const RoleMarkdown = lazy(() => import("./RoleMarkdown"));

/** Whether the two-line clamp hides part of the fragment. */
function useClamped() {
  const ref = useRef<HTMLParagraphElement>(null);
  const [clamped, setClamped] = useState(false);
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () => setClamped(element.scrollHeight > element.clientHeight);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  return { ref, clamped };
}

/**
 * The selected image's ROLE.md under the Image field: a two-line plain-text
 * fragment and, when the file has more, Show role for the full text. Give it a
 * new key for a new image so the full text closes.
 */
export function RoleFragment({ id, text }: { id: string; text: string }) {
  const fullId = useId();
  const fragment = roleFragment(text);
  const { ref, clamped } = useClamped();
  const [open, setOpen] = useState(false);
  return (
    <div className="mt-1 space-y-1">
      <div className="flex flex-col items-start sm:flex-row sm:gap-2">
        <p
          ref={ref}
          id={id}
          className="line-clamp-2 min-w-0 flex-1 text-sm wrap-anywhere text-muted-foreground"
        >
          {fragment.text}
        </p>
        {(fragment.more || clamped) && (
          <button
            type="button"
            aria-expanded={open}
            aria-controls={fullId}
            onClick={() => setOpen((value) => !value)}
            className="h-8 shrink-0 rounded-control text-sm text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none sm:-mt-1.5"
          >
            {open ? "Hide role" : "Show role"}
          </button>
        )}
      </div>
      <div
        id={fullId}
        hidden={!open}
        className="max-h-60 overflow-auto rounded border p-3"
        // A scroll box the keyboard can reach.
        tabIndex={open ? 0 : undefined}
        role={open ? "region" : undefined}
        aria-label={open ? "Role" : undefined}
      >
        {open && (
          <LazyBoundary resetKey="role-markdown">
            <RoleMarkdown text={text} />
          </LazyBoundary>
        )}
      </div>
    </div>
  );
}
