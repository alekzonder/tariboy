import { useId, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";

const LINES = [
  "Completed tasks: tasks done in this period. Their runs, tokens and cost count the whole life of each task.",
  "AI usage in this period: only AI requests made in this period.",
  "Without a task: AI requests that are not linked to a task, for example runs without a selected Goal. Completed tasks do not count them.",
  "Mixed tasks: done tasks that ran on more than one version. Version rows do not count them, so a version comparison stays fair.",
  "— means the value is not known, for example lead time before any task is done. It is not zero.",
  "unknown version: the image version of a run is not recorded.",
  "Data retention removes old runs and assessments. Old tasks can show lower cost and runs, an unknown version, or no quality scores.",
];

/** What the metrics count, closed by default. */
export function AboutNumbers() {
  const [open, setOpen] = useState(false);
  const id = useId();
  const Chevron = open ? ChevronDown : ChevronRight;
  return (
    <div className="text-sm">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 rounded text-muted-foreground hover:text-foreground"
      >
        <Chevron aria-hidden="true" className="size-4" />
        About these numbers
      </button>
      <ul id={id} hidden={!open} className="mt-2 list-disc space-y-1 pl-6 text-muted-foreground">
        {LINES.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
    </div>
  );
}
