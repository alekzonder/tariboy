import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useAgentName } from "@/lib/agent";
import { getAlias, setAlias, ApiError } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";

const err = (e: unknown) => toast.error(e instanceof ApiError ? e.message : String(e));

// AliasEditor renders the agent heading (`Agent: <alias> (<name>)`) and lets the
// operator set, edit, or clear the friendly alias. It self-fetches by agent name.
export function AliasEditor() {
  const name = useAgentName();
  const [alias, setLocal] = useState("");
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");

  useEffect(() => {
    getAlias(name).then((r) => setLocal(r.alias ?? ""), err);
  }, [name]);

  const save = (value: string) => {
    setLocal(value);
    setEditing(false);
    setAlias(name, value).then(() => toast.success("alias saved"), err);
  };

  if (editing) {
    return (
      <div className="flex items-center gap-2">
        <Input
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") save(draft.trim());
            if (e.key === "Escape") setEditing(false);
          }}
          className="h-8 w-56"
          placeholder="alias"
        />
        <Button size="sm" onClick={() => save(draft.trim())}>Save</Button>
        <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>Cancel</Button>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <h1 className="text-lg font-semibold">
        Agent:{" "}
        {alias ? (
          <>
            <span>{alias}</span>{" "}
            <span className="font-mono text-sm text-muted-foreground">({name})</span>
          </>
        ) : (
          <span className="font-mono">{name}</span>
        )}
      </h1>
      <Button size="sm" variant="ghost" onClick={() => { setDraft(alias); setEditing(true); }}>
        {alias ? "edit" : "+ alias"}
      </Button>
      {alias && <Button size="sm" variant="ghost" onClick={() => save("")}>clear</Button>}
    </div>
  );
}
