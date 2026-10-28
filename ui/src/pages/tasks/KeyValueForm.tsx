import { useId, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FIELD } from "./panelStyles";
import type { KeyHint } from "./queueSettingsModel";

/**
 * The names the bound Workflowfile declares for one Environment block: a
 * datalist the key input points at, and a chip for each name the queue has no
 * value for in either block. A chip only fills the key; any key can still be
 * typed.
 */
function KeyHints({
  listId,
  block,
  hints,
  taken,
  onPick,
}: {
  listId: string;
  block: string;
  hints: KeyHint[];
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

/**
 * The Set form of an Environment block: a key, a value, and the names the
 * bound Workflowfile declares for the block. A secret's value is a password
 * input.
 */
export default function KeyValueForm({
  label,
  block,
  secret = false,
  keyName,
  value,
  busy,
  hints,
  taken,
  onKey,
  onValue,
  onSubmit,
}: {
  /** Names the inputs: `Env` gives `Env key` and `Env value`. */
  label: string;
  /** The block's label, which names the chip group. */
  block: string;
  secret?: boolean;
  keyName: string;
  value: string;
  busy: boolean;
  hints: KeyHint[];
  /** Keys with a value as an env var or a secret. */
  taken: string[];
  onKey: (key: string) => void;
  onValue: (value: string) => void;
  onSubmit: (event: React.FormEvent) => void;
}) {
  const listId = useId();
  const valueInput = useRef<HTMLInputElement>(null);
  return (
    <>
      <form
        className="flex min-w-0 flex-wrap items-center gap-1.5"
        autoComplete="off"
        onSubmit={onSubmit}
      >
        <Input
          aria-label={`${label} key`}
          placeholder="KEY"
          list={hints.length > 0 ? listId : undefined}
          value={keyName}
          className={`${FIELD} h-6.5 min-w-25 flex-1`}
          onChange={(event) => onKey(event.target.value)}
        />
        <Input
          ref={valueInput}
          aria-label={`${label} value`}
          type={secret ? "password" : undefined}
          autoComplete={secret ? "new-password" : undefined}
          placeholder="Value"
          value={value}
          className={`${FIELD} h-6.5 min-w-25 flex-1`}
          onChange={(event) => onValue(event.target.value)}
        />
        <Button
          type="submit"
          size="sm"
          className="h-6.5"
          disabled={busy || !keyName.trim() || !value}
        >
          Set
        </Button>
      </form>
      <KeyHints
        listId={listId}
        block={block}
        hints={hints}
        taken={taken}
        onPick={(name) => {
          onKey(name);
          valueInput.current?.focus();
        }}
      />
    </>
  );
}
