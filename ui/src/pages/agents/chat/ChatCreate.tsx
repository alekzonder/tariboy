import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError, apiOn, chatCreateOn, resolveTarget, type ApiTarget } from "@/lib/api";
import type { AgentSummary } from "@/lib/types";
import { chatIdFromTitle } from "./chatModel";

/**
 * Start a chat that belongs to no single agent. The agent whose workspace this
 * was opened from is always in it — that is the chat's starting point — and any
 * number of other agents can join. Membership is what carries delivery, so
 * every agent named here is subscribed to the new chat's channel and is woken
 * by a message in it exactly as it is by one in its own conversation.
 */
export default function ChatCreate({ agent, target, onClose, onCreated }: {
  agent: string;
  target: ApiTarget;
  onClose: () => void;
  onCreated: (chatId: string) => void;
}) {
  const [title, setTitle] = useState("");
  const [others, setOthers] = useState<AgentSummary[]>([]);
  const [picked, setPicked] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const id = chatIdFromTitle(title);

  useEffect(() => {
    void apiOn<{ agents: AgentSummary[] }>(resolveTarget(target), "GET", "/api/agents")
      .then((page) => setOthers((page.agents ?? []).filter((row) => row.name !== agent)))
      .catch(() => setOthers([]));
    // target is derived from the host id and changes identity every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent, target?.baseURL]);

  async function create() {
    if (!id || saving) return;
    setSaving(true);
    try {
      const chat = await chatCreateOn(target, {
        id,
        title: title.trim(),
        participants: [agent, ...picked].map((name) => `agent:${name}`),
      });
      onCreated(chat.chat);
    } catch (error) {
      toast.error(`Could not create the chat: ${error instanceof ApiError ? error.message : String(error)}`);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader><DialogTitle>New chat</DialogTitle></DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="chat-create-title">Title</Label>
            <Input
              id="chat-create-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Team alpha"
            />
            <p className="text-[11.5px] text-muted-foreground">
              {id ? <>Chat id <span className="font-mono">{id}</span></> : "The title becomes the chat id."}
            </p>
          </div>
          <fieldset className="space-y-1.5">
            <legend className="text-[13px] font-medium">Agents</legend>
            <p className="text-[11.5px] text-muted-foreground">
              {agent} is in this chat. Add any others.
            </p>
            <div className="max-h-48 overflow-auto rounded border p-1">
              {others.map((row) => (
                <label key={row.name} className="flex items-center gap-2 px-1.5 py-1 text-[12.5px]">
                  <input
                    type="checkbox"
                    checked={picked.includes(row.name)}
                    onChange={(event) => setPicked((current) => event.target.checked
                      ? [...current, row.name]
                      : current.filter((name) => name !== row.name))}
                  />
                  {row.name}
                </label>
              ))}
              {others.length === 0 && (
                <p className="px-1.5 py-1 text-[12px] text-muted-foreground">No other agents.</p>
              )}
            </div>
          </fieldset>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button onClick={() => void create()} disabled={!id || saving}>Create</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
