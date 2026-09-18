import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { createTask, listTaskQueues } from "@/lib/tasks";
import type { ApiTarget } from "@/lib/api";
import type { ChatMessage } from "@/lib/api";

/** The first line of a message, trimmed to something that reads as a title. */
function titleOf(text: string): string {
  const first = text.split("\n").map((line) => line.trim()).find(Boolean) ?? "";
  return first.length > 120 ? `${first.slice(0, 117)}…` : first;
}

/**
 * Open a task from a message without leaving the chat. The message text
 * becomes the description verbatim — it is already Markdown — and the agent
 * whose thread this is becomes the assignee, because a task opened from its
 * own message is work for it.
 */
export default function ChatTaskFromMessage({ message, agent, target, onClose, onCreated }: {
  message: ChatMessage;
  agent: string;
  target: ApiTarget;
  onClose: () => void;
  onCreated: (key: string) => void;
}) {
  const [queues, setQueues] = useState<string[]>([]);
  const [queue, setQueue] = useState("");
  const [title, setTitle] = useState(() => titleOf(message.text));
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    void listTaskQueues(target)
      .then((page) => {
        const prefixes = (page.queues ?? []).map((row) => row.prefix);
        setQueues(prefixes);
        setQueue((current) => current || prefixes[0] || "");
      })
      .catch(() => toast.error("Could not load the task queues"));
  }, [target]);

  async function create() {
    if (!queue || !title.trim() || saving) return;
    setSaving(true);
    try {
      const task = await createTask({
        queue, title: title.trim(), description: message.text, assignee: `agent:${agent}`,
      }, target);
      onCreated(task.key);
    } catch (error) {
      toast.error(`Could not create the task: ${error instanceof Error ? error.message : String(error)}`);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader><DialogTitle>Create a task from this message</DialogTitle></DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="chat-task-queue">Queue</Label>
            <Select value={queue} onValueChange={setQueue}>
              <SelectTrigger id="chat-task-queue"><SelectValue placeholder="Select a queue" /></SelectTrigger>
              <SelectContent>
                {queues.map((prefix) => <SelectItem key={prefix} value={prefix}>{prefix}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="chat-task-title">Title</Label>
            <Input id="chat-task-title" value={title} onChange={(event) => setTitle(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="chat-task-description">Description</Label>
            <Textarea id="chat-task-description" readOnly rows={5} value={message.text} className="font-mono text-[11.5px]" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button onClick={() => void create()} disabled={saving || !queue || !title.trim()}>Create task</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
