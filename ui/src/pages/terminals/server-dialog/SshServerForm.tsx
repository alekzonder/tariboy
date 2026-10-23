import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { HostOperationProgress } from "./HostOperationProgress";
import { idleOperation } from "./hostOperation";
import type { ServerForm } from "./useServerForm";

export function SshServerForm({
  sshAlias, setSshAlias, editing, operation, sshSetup, setSshSetup,
  updateOperation, displayHost, output, prompt, reply, setReply, sendReply,
}: Pick<ServerForm,
  "sshAlias" | "setSshAlias" | "editing" | "operation" | "sshSetup" | "setSshSetup" |
  "updateOperation" | "displayHost" | "output" | "prompt" | "reply" | "setReply" | "sendReply"
>) {
  return (
    <>
      <div className="space-y-1">
        <Label htmlFor="add-host-alias">SSH alias</Label>
        <Input
          id="add-host-alias"
          value={sshAlias}
          onChange={(event) => setSshAlias(event.target.value)}
          placeholder="the name from ~/.ssh/config"
          autoComplete="off"
          className="h-8"
        />
        <p className="text-xs text-muted-foreground">
          Uses your system OpenSSH config and ssh-agent.
        </p>
      </div>

      {!editing && (
        <fieldset className="space-y-2 rounded-lg border p-3" disabled={operation.status === "running"}>
          <legend className="px-1 text-sm font-medium">Setup</legend>
          <label className="flex items-start gap-2 text-sm">
            <input
              type="radio"
              aria-label="Install Tariboy"
              name="ssh-setup"
              value="install"
              checked={sshSetup === "install"}
              onChange={() => {
                setSshSetup("install");
                updateOperation(idleOperation("provision"));
              }}
            />
            <span>
              <span className="block font-medium">Install Tariboy</span>
              <span className="text-xs text-muted-foreground">Install and start the bundled version.</span>
            </span>
          </label>
          <label className="flex items-start gap-2 text-sm">
            <input
              type="radio"
              aria-label="Connect to existing Tariboy"
              name="ssh-setup"
              value="connect"
              checked={sshSetup === "connect"}
              onChange={() => {
                setSshSetup("connect");
                updateOperation(idleOperation("connect"));
              }}
            />
            <span>
              <span className="block font-medium">Connect to existing Tariboy</span>
              <span className="text-xs text-muted-foreground">Connect without installing or restarting Tariboy.</span>
            </span>
          </label>
        </fieldset>
      )}

      <HostOperationProgress

        operation={operation}

        displayHost={displayHost}

        output={output}

      />

      {prompt && (
        <div className="space-y-2 rounded border p-2">
          <div className="text-sm">{prompt.text || "SSH authentication reply required"}</div>
          <Label htmlFor="host-prompt-reply">SSH reply</Label>
          <div className="flex gap-2">
            <Input
              id="host-prompt-reply"
              type="password"
              value={reply}
              onChange={(event) => setReply(event.target.value)}
              autoComplete="one-time-code"
            />
            <Button type="button" onClick={() => void sendReply()}>
              Send reply
            </Button>
          </div>
        </div>
      )}

    </>
  );
}
