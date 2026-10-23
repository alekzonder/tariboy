import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { removedSecrets } from "./commits";
import { DraftField, FieldFrame } from "./ConfigControls";
import { fieldOfKey, type ConfigField } from "./registry";
import type { BlockProps } from "./ConfigurationBlocks";

const field = (key: string) => fieldOfKey(key) as ConfigField;

export function EnvironmentBlock({ draft, matched, sources }: BlockProps) {
  const { script, secrets } = sources;
  const removed = removedSecrets(draft.values["secrets-remove"] ?? "");
  const toggle = (key: string) => {
    const next = removed.includes(key) ? removed.filter((k) => k !== key) : [...removed, key];
    draft.setField("secrets-remove", next.join("\n"));
  };
  const listField = field("secrets-remove");
  return (
    <>
      {script.error && <p role="alert" className="text-sm text-destructive">Shell script unavailable: {script.error}</p>}
      <DraftField field={field("shell-script")} draft={draft} matched={matched.has("shell-script")} />
      {/* SECURITY: secrets are write-only. The list holds key names only
          (the daemon never returns values); a new value lives only in the
          page draft while typing, is sent in a POST body, never in a URL, and
          is dropped once the daemon acknowledges it. */}
      {secrets.error && <p role="alert" className="text-sm text-destructive">Secrets unavailable: {secrets.error}</p>}
      <FieldFrame field={listField} draft={draft} matched={matched.has("secrets-remove")}>
        <div id="secrets-remove" tabIndex={-1} className="flex flex-wrap items-center gap-1.5">
          {(secrets.value ?? []).map((key) => {
            const pending = removed.includes(key);
            return (
              <Badge key={key} variant={pending ? "destructive" : "secondary"} className="gap-1">
                <span className={cn(pending && "line-through")}>{key}</span>
                {pending && <span>Will be removed</span>}
                <button
                  type="button"
                  className="ml-1 text-xs hover:text-destructive"
                  aria-label={pending ? `Keep ${key}` : `Remove ${key}`}
                  onClick={() => toggle(key)}
                >
                  {pending ? "Undo" : "×"}
                </button>
              </Badge>
            );
          })}
          {secrets.value?.length === 0 && (
            <span className="text-sm text-muted-foreground">Add a secret key and value to make it available to the agent.</span>
          )}
        </div>
      </FieldFrame>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <DraftField field={field("secret-key")} draft={draft} matched={matched.has("secret-key")} />
        <DraftField field={field("secret-value")} draft={draft} matched={matched.has("secret-value")} />
      </div>
    </>
  );
}
