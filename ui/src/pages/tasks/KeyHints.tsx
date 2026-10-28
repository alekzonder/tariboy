import type { KeyHint } from "./queueSettingsModel";

/**
 * The names the bound Workflowfile declares for one Environment block: a
 * datalist the key input points at, and a chip for each name the queue has no
 * value for in either block. A chip only fills the key; any key can still be
 * typed.
 */
export default function KeyHints({
  listId,
  block,
  hints,
  taken,
  onPick,
}: {
  listId: string;
  /** The block's label, which names the chip group. */
  block: string;
  hints: KeyHint[];
  /** Keys with a value as an env var or a secret. */
  taken: string[];
  onPick: (name: string) => void;
}) {
  if (hints.length === 0) return null;
  const unset = hints.filter((hint) => !taken.includes(hint.name));
  return (
    <>
      <datalist id={listId}>
        {hints.map((hint) => (
          <option key={hint.name} value={hint.name} />
        ))}
      </datalist>
      {unset.length > 0 && (
        <div
          role="group"
          aria-label={`${block} declared in Workflowfile`}
          className="flex min-w-0 flex-wrap items-center gap-1 text-ui text-muted-foreground"
        >
          <span>Declared in Workflowfile:</span>
          {unset.map((hint) => (
            <button
              key={hint.name}
              type="button"
              title={hint.title}
              className="h-5.5 rounded-control bg-muted px-2 font-mono text-meta text-foreground hover:bg-accent"
              onClick={() => onPick(hint.name)}
            >
              {hint.name}
            </button>
          ))}
        </div>
      )}
    </>
  );
}
