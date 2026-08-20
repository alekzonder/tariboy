import { useState } from "react";
import { toast } from "sonner";
import { apiPost, ApiError } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { SendFilesButton } from "@/components/SendFilesButton";

// InboxComposer writes a single message into this agent's inbox channel
// (agent:<name>:inbox) so the next iteration picks it up. Fixed type="message";
// the server stamps source="operator".
export function InboxComposer({ name }: { name: string }) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);

  const send = async () => {
    const body = text.trim();
    if (!body || busy) return;
    setBusy(true);
    try {
      await apiPost("/api/messages", { channel: `agent:${name}:inbox`, type: "message", text: body });
      setText("");
      toast.success("sent to inbox");
    } catch (e) {
      toast.error(`send failed: ${e instanceof ApiError ? e.message : String(e)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex items-center gap-2">
      <span className="text-xs text-muted-foreground">inbox</span>
      <Input
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="message to inbox…"
        className="h-8 flex-1"
        onKeyDown={(e) => { if (e.key === "Enter") void send(); }}
      />
      {/* Uploaded file paths are appended to the message so the agent sees them. */}
      <SendFilesButton
        name={name}
        onUploaded={(paths) =>
          setText((t) => (t.trim() ? `${t} ${paths.join(" ")}` : paths.join(" ")))
        }
      />
      <Button size="sm" onClick={() => void send()} disabled={busy || text.trim() === ""}>Send</Button>
    </div>
  );
}
