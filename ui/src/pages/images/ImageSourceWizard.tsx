import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError } from "@/lib/api";
import { createImageSource } from "@/lib/imageSources";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

export default function ImageSourceWizard() {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  const submit = async () => {
    setPending(true);
    setError("");
    try {
      const source = await createImageSource({
        name: name.trim(),
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
