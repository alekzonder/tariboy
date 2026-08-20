import { useEffect, useState } from "react";
import { useAgentName } from "@/lib/agent";
import { agentContextGet, agentContextSet } from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

function ContextEditor({ name }: { name: string }) {
  const [text, setText] = useState("");
  useEffect(() => {
    void agentContextGet(name).then((r) => setText(r.context)).catch(() => setText(""));
  }, [name]);
  return (
    <Card>
      <CardHeader className="pb-2"><CardTitle className="text-base">Context</CardTitle></CardHeader>
      <CardContent className="space-y-2">
        <Textarea value={text} onChange={(e) => setText(e.target.value)} rows={8} className="font-mono text-xs" />
        <Button size="sm" onClick={() => guard("context", () => agentContextSet(name, text))}>Save context</Button>
      </CardContent>
    </Card>
  );
}

export default function AgentContext() {
  const name = useAgentName();
  if (!name) return null;
  return (
    <div className="grid gap-4">
      <ContextEditor key={`ctx-${name}`} name={name} />
    </div>
  );
}
