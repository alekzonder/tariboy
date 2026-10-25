import type { ImageSummary } from "@/lib/api";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { ImageCombobox } from "@/components/AgentFormFields";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { AgentAdvancedFields } from "./AgentAdvancedFields";
import type { AgentDraft, RowStatus } from "./groupWizardModel";

interface Props {
  agent: AgentDraft;
  index: number;
  isLeader: boolean;
  status: RowStatus | undefined;
  images: ImageSummary[];
  allowsPlugins: boolean;
  canRemove: boolean;
  onSetLeader: () => void;
  onRemove: () => void;
  onPatch: (up: Partial<AgentDraft>) => void;
}

function RowStatusBadge({ status: st }: { status: RowStatus | undefined }) {
  if (st?.kind === "creating") return <Badge variant="secondary">creating…</Badge>;
  if (st?.kind === "created") return <Badge>created {st.name}</Badge>;
  if (st?.kind === "failed")
    return (
      <Badge variant="destructive" title={st.reason}>
        failed: {st.reason}
      </Badge>
    );
  return null;
}

export function GroupWizardAgentRow(props: Props) {
  const { agent: a, index: i, isLeader, images, onPatch: patch } = props;
  return (
    <div data-slot="agent-row" className="space-y-3 rounded-lg border p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 text-sm">
          <label className="flex items-center gap-1.5">
            <input
              type="radio"
              name="leader"
              aria-label={`leader ${i}`}
              checked={isLeader}
              onChange={props.onSetLeader}
            />
            <span className="text-muted-foreground">{isLeader ? "lead" : "worker"}</span>
          </label>
          <RowStatusBadge status={props.status} />
        </div>
        {props.canRemove && (
          <Button variant="ghost" size="sm" type="button" onClick={props.onRemove}>
            Remove agent
          </Button>
        )}
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1">
          <Label htmlFor={`img-${a.id}`}>image *</Label>
          <ImageCombobox
            id={`img-${a.id}`}
            ariaLabel={`image ${i}`}
            images={images}
            value={a.image}
            onChange={(v) => patch({ image: v })}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`name-${a.id}`}>name</Label>
          <Input
            id={`name-${a.id}`}
            value={a.name}
            onChange={(e) => patch({ name: e.target.value })}
            placeholder="empty = generated"
            className="h-8"
          />
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id={`override-${a.id}`}
          checked={a.override}
          onCheckedChange={(v) => patch({ override: v })}
        />
        <Label htmlFor={`override-${a.id}`}>override cwd</Label>
      </div>
      {a.override && (
        <PathAutocomplete
          value={a.cwd}
          onChange={(v) => patch({ cwd: v })}
          placeholder="this agent's cwd"
          aria-label={`cwd ${i}`}
        />
      )}

      <AgentAdvancedFields
        agent={a}
        index={i}
        allowsPlugins={props.allowsPlugins}
        onPatch={patch}
      />
    </div>
  );
}
