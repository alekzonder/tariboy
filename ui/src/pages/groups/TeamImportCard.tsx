import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { TeamImportPreview } from "./TeamImportPreview";
import { TeamImportProgress } from "./TeamImportProgress";
import type { useTeamImport } from "./useTeamImport";

interface Props {
  teamImport: ReturnType<typeof useTeamImport>;
}

export function TeamImportCard({ teamImport }: Props) {
  const [yaml, setYaml] = useState("");
  const { preview, operation, operationRef } = teamImport;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Import team</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <Textarea
          aria-label="Import compose YAML"
          value={yaml}
          onChange={(event) => setYaml(event.target.value)}
          placeholder="Paste tariboy-compose.yaml"
          className="min-h-32 w-full rounded border bg-background p-2 font-mono text-xs"
        />
        <Button
          size="sm"
          disabled={!yaml.trim()}
          onClick={() => teamImport.previewYaml(yaml, () => setYaml(""))}
        >
          Preview YAML
        </Button>
        <div>
          <label className="text-sm font-medium" htmlFor="team-archive">
            Portable team archive (compose only; transfer images separately)
          </label>
          <Input
            id="team-archive"
            aria-label="Import team archive"
            type="file"
            accept=".gz,.tgz,application/gzip"
            className="mt-1"
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (!file) return;
              teamImport.uploadArchive(file);
            }}
          />
        </div>
        {preview && (
          <TeamImportPreview
            preview={preview}
            onChange={teamImport.setPreview}
            onConfirm={() => teamImport.confirm(preview)}
          />
        )}
        {operation && (
          <TeamImportProgress
            operation={operation}
            canRetry={operationRef !== null}
            onRetry={() => operationRef && teamImport.retry(operationRef, operation)}
          />
        )}
      </CardContent>
    </Card>
  );
}
