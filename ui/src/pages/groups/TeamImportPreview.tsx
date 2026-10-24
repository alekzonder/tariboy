import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { ImportPreview } from "./teamImportModel";

interface Props {
  preview: ImportPreview;
  onChange: (preview: ImportPreview) => void;
  onConfirm: () => void;
}

function PreviewImages({ preview, onChange }: Omit<Props, "onConfirm">) {
  const setResolved = (index: number, resolved: string) =>
    onChange({
      ...preview,
      images: preview.images.map((candidate, candidateIndex) =>
        candidateIndex === index ? { ...candidate, resolved } : candidate,
      ),
    });
  return (
    <div className="space-y-1">
      Images: {preview.images.length === 0 && "none"}
      {preview.images.map((image, index) => (
        <div key={image.original}>
          <Input
            aria-label={`Imported team image ref ${index}`}
            value={image.resolved}
            className="h-8 font-mono"
            onChange={(event) => setResolved(index, event.target.value)}
          />
          <span className={image.conflict ? "text-destructive" : "text-muted-foreground"}>
            {image.action}
            {image.message ? ` — ${image.message}` : ""}
          </span>
        </div>
      ))}
    </div>
  );
}

export function TeamImportPreview({ preview, onChange, onConfirm }: Props) {
  return (
    <div className="space-y-2 rounded border p-3 text-sm">
      <div>
        Team: <strong>{preview.team}</strong>
      </div>
      <PreviewImages preview={preview} onChange={onChange} />
      {preview.agents.map((agent) => (
        <div
          key={agent.name}
          className={agent.conflict ? "text-destructive" : "text-muted-foreground"}
        >
          agent {agent.name}: {agent.action}
          {agent.message ? ` — ${agent.message}; edit YAML to rename it` : ""}
        </div>
      ))}
      {preview.groups.map((group) => (
        <div
          key={group.name}
          className={group.conflict ? "text-destructive" : "text-muted-foreground"}
        >
          team {group.name}: {group.action}
          {group.message ? ` — ${group.message}` : ""}
        </div>
      ))}
      {preview.groups.some((group) => group.conflict) && (
        <label className="flex items-center gap-2">
          <input
            aria-label="Update existing team"
            type="checkbox"
            checked={preview.updateExisting}
            onChange={(event) => onChange({ ...preview, updateExisting: event.target.checked })}
          />
          Update the existing team additively (members not present in YAML are preserved)
        </label>
      )}
      <Textarea
        aria-label="Imported team compose YAML"
        value={preview.yaml}
        onChange={(event) => onChange({ ...preview, yaml: event.target.value })}
        className="max-h-48 min-h-32 w-full overflow-auto rounded bg-muted p-2 font-mono text-xs"
      />
      <Button size="sm" aria-label="Confirm and import team" onClick={onConfirm}>
        Confirm and import team
      </Button>
    </div>
  );
}
