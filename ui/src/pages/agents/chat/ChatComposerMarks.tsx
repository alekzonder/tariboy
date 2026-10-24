import type { RefObject } from "react";
import { MARKS, applyMark, type ComposerMark } from "./chatComposerModel";

/** The Markdown mark buttons: each one rewrites the draft around the selection
 *  and puts the caret back where typing continues. */
export function ChatComposerMarks({
  box,
  value,
  onChange,
}: {
  box: RefObject<HTMLTextAreaElement | null>;
  value: string;
  onChange: (value: string) => void;
}) {
  function apply(mark: ComposerMark) {
    const area = box.current;
    if (!area) return;
    const start = area.selectionStart ?? value.length;
    const end = area.selectionEnd ?? start;
    const { next, caret } = applyMark({ value, start, end, mark });
    onChange(next);
    requestAnimationFrame(() => {
      area.focus();
      area.setSelectionRange(caret, caret);
    });
  }

  return MARKS.map((mark) => (
    <button
      key={mark.title}
      type="button"
      aria-label={mark.title}
      title={mark.title}
      onClick={() => apply(mark)}
      className="grid h-5.5 min-w-5.5 place-items-center rounded-sm px-1.5 font-mono text-meta font-semibold text-muted-foreground hover:bg-accent hover:text-foreground"
    >
      {mark.label}
    </button>
  ));
}
