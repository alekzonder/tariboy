import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError } from "@/lib/api";
import { createImageSource } from "@/lib/imageSources";
import { HARNESSES } from "@/components/AgentFormFields";
import { EFFORT_PRESETS } from "@/components/ComboField";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

export default function ImageSourceWizard() {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [from, setFrom] = useState("");
  const [harness, setHarness] = useState("claude");
  const [model, setModel] = useState("");
  const [effort, setEffort] = useState("");
  const [interactive, setInteractive] = useState(true);
  const [capabilities, setCapabilities] = useState("");
  const [prompt, setPrompt] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  const submit = async () => {
    setPending(true);
    setError("");
    try {
      const source = await createImageSource({
        name: name.trim(),
        from: from.trim() || undefined,
        harness,
        model: model.trim() || undefined,
        effort: effort || undefined,
        interactive,
        capabilities: capabilities
          .split(",")
          .map((value) => value.trim())
          .filter(Boolean),
        prompt,
      });
      navigate(`/images/sources/${encodeURIComponent(source.name)}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="mx-auto max-w-3xl space-y-4 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold">New Image</h1>
          <p className="text-sm text-muted-foreground">
            Create a managed source project on the selected host.
          </p>
        </div>
        <Button variant="ghost" onClick={() => navigate("/images")}>Cancel</Button>
      </div>
      <Card>
        <CardHeader><CardTitle>Image source</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="source-name">name *</Label>
              <Input
                id="source-name"
                aria-label="name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="reviewer"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="source-parent">parent image</Label>
              <Input
                id="source-parent"
                aria-label="parent image"
                value={from}
                onChange={(event) => setFrom(event.target.value)}
                placeholder="optional name:tag"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="source-harness">harness</Label>
              <select
                id="source-harness"
                aria-label="harness"
                value={harness}
                onChange={(event) => setHarness(event.target.value)}
                className="h-9 w-full rounded-md border bg-background px-3 text-sm"
              >
                {HARNESSES.map((value) => <option key={value}>{value}</option>)}
              </select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="source-model">model</Label>
              <Input
                id="source-model"
                aria-label="model"
                value={model}
                onChange={(event) => setModel(event.target.value)}
                placeholder="harness default"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="source-effort">effort</Label>
              <select
                id="source-effort"
                aria-label="effort"
                value={effort}
                onChange={(event) => setEffort(event.target.value)}
                className="h-9 w-full rounded-md border bg-background px-3 text-sm"
              >
                <option value="">default</option>
                {EFFORT_PRESETS.map((value) => <option key={value}>{value}</option>)}
              </select>
            </div>
            <div className="flex items-center gap-2 pt-6">
              <Switch
                id="source-interactive"
                aria-label="interactive default"
                checked={interactive}
                onCheckedChange={setInteractive}
              />
              <Label htmlFor="source-interactive">interactive default</Label>
            </div>
          </div>
          <div className="space-y-1">
            <Label htmlFor="source-capabilities">capabilities</Label>
            <Input
              id="source-capabilities"
              aria-label="capabilities"
              value={capabilities}
              onChange={(event) => setCapabilities(event.target.value)}
              placeholder="context, status, filesystem"
            />
            <p className="text-xs text-muted-foreground">Comma-separated capability names.</p>
          </div>
          <div className="space-y-1">
            <Label htmlFor="source-prompt">initial prompt</Label>
            <Textarea
              id="source-prompt"
              aria-label="initial prompt"
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              placeholder="Describe this agent's role and operating instructions."
              className="min-h-40"
            />
          </div>
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          <Button disabled={!name.trim() || pending} onClick={() => void submit()}>
            {pending ? "Creating…" : "Create source"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
