import { useEffect, useState } from "react";
import { useAgentName } from "@/lib/agent";
import {
  ApiError,
  agentPromptGet, agentUserPromptGet, agentUserPromptSet,
  type AgentPromptLayer,
} from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

function UserPromptEditor({ name }: { name: string }) {
  const [text, setText] = useState("");
  useEffect(() => {
    void agentUserPromptGet(name).then((r) => setText(r.user_prompt)).catch(() => setText(""));
  }, [name]);
  return (
    <Card>
      <CardHeader className="pb-2"><CardTitle className="text-base">User prompt</CardTitle></CardHeader>
      <CardContent className="space-y-2">
        <Textarea value={text} onChange={(e) => setText(e.target.value)} rows={6} className="font-mono text-xs" />
        <div className="flex gap-2">
          <Button size="sm" onClick={() => guard("user-prompt", () => agentUserPromptSet(name, text))}>Save prompt</Button>
          <Button size="sm" variant="secondary"
            onClick={() => { setText(""); void guard("user-prompt", () => agentUserPromptSet(name, "")); }}>
            Clear
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function PromptPreview({ name }: { name: string }) {
  const [prompt, setPrompt] = useState("");
  const [layers, setLayers] = useState<AgentPromptLayer[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    void agentPromptGet(name)
      .then((r) => { setPrompt(r.prompt); setLayers(r.layers ?? []); setError(""); })
      .catch((e) => setError(e instanceof ApiError ? e.message : String(e)));
  }, [name]);
  return (
    <Card>
      <CardHeader className="pb-2"><CardTitle className="text-base">Prompt preview</CardTitle></CardHeader>
      <CardContent className="space-y-2">
        {error ? (
          <p className="text-xs text-muted-foreground">{error}</p>
        ) : (
          <>
            <Textarea value={prompt} readOnly rows={12} className="font-mono text-xs" />
            {layers.length > 0 && (
              <ul className="text-xs text-muted-foreground">
                {layers.map((layer, index) => {
                  if ("name" in layer) {
                    return <li key={`${index}-${layer.name}`}>{layer.name} — {layer.sha256.slice(0, 12)}</li>;
                  }
                  if (layer.kind === "runtime") {
                    return <li key={`${index}-runtime-${layer.runtime}`}>runtime — {layer.runtime}</li>;
                  }
                  const label = layer.source ?? layer.archive_path ?? `file ${index + 1}`;
                  return (
                    <li key={`${index}-file-${label}`}>
                      {label}{layer.sha256 ? ` — ${layer.sha256.slice(0, 12)}` : ""}
                    </li>
                  );
                })}
              </ul>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

export default function AgentPrompt() {
  const name = useAgentName();
  if (!name) return null;
  return (
    <div className="grid gap-4">
      <PromptPreview key={`pp-${name}`} name={name} />
      <UserPromptEditor key={`up-${name}`} name={name} />
    </div>
  );
}
