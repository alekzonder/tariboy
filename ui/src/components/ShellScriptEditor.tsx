import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

interface Props {
  title: string;
  description: string;
  load: () => Promise<string>;
  save: (script: string) => Promise<unknown>;
}

export function ShellScriptEditor({ title, description, load, save }: Props) {
  const [saved, setSaved] = useState("");
  const [draft, setDraft] = useState("");
  const [loaded, setLoaded] = useState<(() => Promise<string>) | null>(null);
  const [errorState, setErrorState] = useState<{ load: Props["load"]; message: string } | null>(null);
  const ready = loaded === load;
  const error = errorState?.load === load ? errorState.message : "";
  const id = title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");

  useEffect(() => {
    let current = true;
    void load().then(
      (script) => {
        if (!current) return;
        setSaved(script);
        setDraft(script);
        setLoaded(() => load);
      },
      (cause) => {
        if (current) {
          setErrorState({ load, message: cause instanceof Error ? cause.message : String(cause) });
        }
      },
    );
    return () => {
      current = false;
    };
  }, [load]);

  const saveDraft = async () => {
    if (!ready) return;
    setErrorState(null);
    try {
      await save(draft);
      setSaved(draft);
    } catch (cause) {
      setErrorState({ load, message: cause instanceof Error ? cause.message : String(cause) });
    }
  };

  return (
    <Card aria-busy={!ready}>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <Label htmlFor={id}>{title}</Label>
        <Textarea
          id={id}
          value={draft}
          onChange={(event) => { if (ready) setDraft(event.target.value); }}
          disabled={!ready}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? `${id}-error` : undefined}
          className="min-h-40 font-mono"
        />
        {error ? (
          <p
            id={`${id}-error`}
            role="alert"
            className="text-sm text-destructive"
          >
            {error}
          </p>
        ) : null}
      </CardContent>
      <CardFooter className="justify-end gap-2">
        {draft !== saved ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setDraft(saved);
              setErrorState(null);
            }}
          >
            Discard changes
          </Button>
        ) : null}
        <Button
          type="button"
          onClick={() => void saveDraft()}
          aria-label={`Save ${title.toLowerCase()}`}
          disabled={!ready}
        >
          Save
        </Button>
      </CardFooter>
    </Card>
  );
}
