import { useState } from "react";
import { toast } from "sonner";
import { agentPost, apiDelete, agentApiPath, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

function useAction(refresh?: () => void) {
  return async (label: string, fn: () => Promise<unknown>) => {
    try {
      await fn();
      toast.success(`${label} ok`);
      refresh?.();
    } catch (e) {
      const m = e instanceof ApiError ? e.message : String(e);
      toast.error(`${label} failed: ${m}`);
    }
  };
}

// RunStateActions is the lifecycle trio on its own. The Configuration page's
// run-state strip mounts THIS rather than AgentControls, so it shares these
// handlers instead of hand-rolling a second start/stop control — and without
// carrying Kill, Remove and the one-shot exec prompt onto that page.
export function RunStateActions({ name, refresh }: { name: string; refresh?: () => void }) {
  const run = useAction(refresh);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button size="sm" onClick={() => run("start", () => agentPost(name, "start"))}>Start</Button>
      <Button size="sm" variant="secondary" onClick={() => run("stop", () => agentPost(name, "stop"))}>Stop</Button>
      <Button size="sm" variant="secondary" onClick={() => run("restart", () => agentPost(name, "restart"))}>Restart</Button>
    </div>
  );
}

export function AgentControls({ name, refresh }: { name: string; refresh?: () => void }) {
  const run = useAction(refresh);
  const [prompt, setPrompt] = useState("");

  return (
    <div className="flex flex-wrap items-center gap-2 rounded border p-2">
      <RunStateActions name={name} refresh={refresh} />

      <AlertDialog>
        <AlertDialogTrigger asChild>
          <Button size="sm" variant="destructive">Kill</Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Kill current iteration?</AlertDialogTitle>
            <AlertDialogDescription>
              This kills the running iteration of <span className="font-mono">{name}</span> via its shim.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => run("kill", () => agentPost(name, "kill"))}>Kill</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog>
        <AlertDialogTrigger asChild>
          <Button size="sm" variant="destructive">Remove</Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove agent {name}?</AlertDialogTitle>
            <AlertDialogDescription>
              This force-removes the agent and its state. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() =>
                run("remove", () => apiDelete(`${agentApiPath(name, "")}?force=true`))
              }
            >
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <div className="ml-auto flex items-center gap-2">
        <Input
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="one-shot exec prompt (optional)"
          className="h-8 w-64"
        />
        <Button
          size="sm"
          onClick={() =>
            run("exec", () => agentPost(name, "exec", prompt ? { prompt } : undefined)).then(() => setPrompt(""))
          }
        >
          Exec
        </Button>
      </div>
    </div>
  );
}
