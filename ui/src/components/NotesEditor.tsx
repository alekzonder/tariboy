import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useAgentName } from "@/lib/agent";
import { getNotes, setNotes, ApiError } from "@/lib/api";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

const err = (e: unknown) => toast.error(e instanceof ApiError ? e.message : String(e));

// NotesEditor is a free-form per-agent notes textarea, saved on blur (only when
// the text actually changed). It self-fetches by agent name.
export function NotesEditor() {
  const name = useAgentName();
  const [notes, setLocal] = useState("");
  const [loaded, setLoaded] = useState("");

  useEffect(() => {
    getNotes(name).then((r) => { setLocal(r.notes ?? ""); setLoaded(r.notes ?? ""); }, err);
  }, [name]);

  const saveOnBlur = () => {
    if (notes === loaded) return;
    setLoaded(notes);
    setNotes(name, notes).then(() => toast.success("notes saved"), err);
  };

  return (
    <Card>
      <CardHeader className="pb-2"><CardTitle className="text-base">Notes</CardTitle></CardHeader>
      <CardContent>
        <Textarea
          value={notes}
          onChange={(e) => setLocal(e.target.value)}
          onBlur={saveOnBlur}
          className="min-h-24 font-mono text-sm"
          placeholder="Free-form notes for this agent…"
        />
      </CardContent>
    </Card>
  );
}
