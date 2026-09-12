import { useEffect, useRef, type ReactNode } from "react";
import { FullAuditLog } from "tariboy-ui";

// FullAuditLog pages one agent's audit history: recent events first, older
// pages in on scroll-up, newer ones appended by a 3 s poll plus an SSE tail. A
// type/text filter switches the pane into a server-side query view.
//
// Every one of those reads fails in a preview, so the component lands in its
// real empty state — the filter toolbar (type datalist + full-text search +
// AuditExportActions) above a bordered, muted scroll pane reading "No events."
// The component is h-full, so the preview gives it a height to fill.

function useType(selector: string, value: string) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const input = ref.current?.querySelector<HTMLInputElement>(selector);
      if (!input) return;
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
      setter?.call(input, value);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    }, 120);
    return () => window.clearTimeout(timer);
  }, [selector, value]);
  return ref;
}

const Pane = ({ children }: { children: ReactNode }) => (
  <div style={{ height: 380, width: 760 }}>{children}</div>
);

export const EmptyLog = () => (
  <Pane>
    <FullAuditLog name="builder" />
  </Pane>
);

// Filtered mode: a filter reveals the Clear button and the match-count line, and
// pauses the paging/tail machinery. The server-side scan returns nothing here,
// so the pane shows the "No matching events." wording, not the unfiltered one.
export const Filtered = () => {
  const ref = useType('input[aria-label="Search text"]', "TB-142");
  return (
    <div ref={ref}>
      <Pane>
        <FullAuditLog name="reviewer" />
      </Pane>
    </div>
  );
};

export const FilteredByType = () => {
  const ref = useType('input[aria-label="Filter by type"]', "tool_use");
  return (
    <div ref={ref}>
      <Pane>
        <FullAuditLog name="docs-bot" />
      </Pane>
    </div>
  );
};
