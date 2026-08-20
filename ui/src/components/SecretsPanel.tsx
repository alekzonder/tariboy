import { useEffect, useState } from "react";
import { toast } from "sonner";
import { agentGet, agentPost, apiDelete, agentApiPath, ApiError } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";

// SECURITY: this panel is write-only. It lists secret KEY NAMES only
// (secret.ls returns names, never values) and lets the operator set/remove.
// A secret VALUE lives only in local component state while typing and is sent
// in a POST body; it is never rendered back, never put in a URL/query, and is
// cleared immediately after a successful store.
export function SecretsPanel({ name }: { name: string }) {
  const [keys, setKeys] = useState<string[]>([]);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");

  const load = () => agentGet<{ keys: string[] }>(name, "secrets").then((r) => setKeys(r.keys)).catch(() => setKeys([]));
  useEffect(() => { if (name) void load(); }, [name]);

  const store = async () => {
    try {
      await agentPost(name, "secrets", { key, value });
      setKey("");
      setValue(""); // clear the value from memory immediately
      toast.success("secret stored");
      void load();
    } catch (e) { toast.error(`store failed: ${e instanceof ApiError ? e.message : String(e)}`); }
  };
  const remove = (k: string) =>
    apiDelete(`${agentApiPath(name, "secrets")}?key=${encodeURIComponent(k)}`)
      .then(() => { toast.success("removed"); void load(); })
      .catch((e) => toast.error(`rm failed: ${e instanceof ApiError ? e.message : String(e)}`));

  return (
    <Card>
      <CardHeader className="pb-2"><CardTitle className="text-base">Secrets (write-only)</CardTitle></CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-1">
          {keys.map((k) => (
            <Badge key={k} variant="secondary" className="gap-1">
              {k}
              <button className="ml-1 text-xs hover:text-destructive" onClick={() => void remove(k)} aria-label={`remove ${k}`}>×</button>
            </Badge>
          ))}
          {/* An empty list invites the next action instead of only reporting
              the absence of one. */}
          {keys.length === 0 && (
            <span className="text-sm text-muted-foreground">
              Add a secret key and value to make it available to the agent.
            </span>
          )}
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <div className="space-y-1.5">
            <Label htmlFor="secret-key">Secret key</Label>
            <Input id="secret-key" value={key} onChange={(e) => setKey(e.target.value)} placeholder="KEY" className="h-8 w-40" />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="secret-value">Secret value</Label>
            <Input id="secret-value" type="password" value={value} onChange={(e) => setValue(e.target.value)} placeholder="value (never shown)" className="h-8 w-56" />
          </div>
          <Button size="sm" onClick={() => void store()}>Store secret</Button>
        </div>
        <p className="text-xs text-muted-foreground">Takes effect on the next iteration.</p>
      </CardContent>
    </Card>
  );
}
