import { EFFORT_PRESETS } from "@/components/ComboField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { AgentDraft } from "./groupWizardModel";

type Patch = (up: Partial<AgentDraft>) => void;

interface RowProps {
  agent: AgentDraft;
  index: number;
  onPatch: Patch;
}

function AgentEnvRows({ agent: a, index: i, onPatch: patch }: RowProps) {
  const setRow = (j: number, up: { key?: string; value?: string }) =>
    patch({ envRows: a.envRows.map((r, k) => (k === j ? { ...r, ...up } : r)) });
  return (
    <div className="space-y-2">
      <Label>env (K=V)</Label>
      {a.envRows.map((row, j) => (
        <div key={j} className="flex items-center gap-2">
          <Input
            aria-label={`env key ${i}.${j}`}
            value={row.key}
            onChange={(e) => setRow(j, { key: e.target.value })}
            placeholder="KEY"
            className="h-8 w-40"
          />
          <span className="text-muted-foreground">=</span>
          <Input
            aria-label={`env value ${i}.${j}`}
            value={row.value}
            onChange={(e) => setRow(j, { value: e.target.value })}
            placeholder="value"
            className="h-8 w-48"
          />
          <Button
            variant="ghost"
            size="sm"
            type="button"
            onClick={() => patch({ envRows: a.envRows.filter((_, k) => k !== j) })}
          >
            Remove
          </Button>
        </div>
      ))}
      <Button
        variant="outline"
        size="sm"
        type="button"
        onClick={() => patch({ envRows: [...a.envRows, { key: "", value: "" }] })}
      >
        + Add env
      </Button>
    </div>
  );
}

function AgentPluginsField({ agent: a, onPatch: patch }: Omit<RowProps, "index">) {
  const addDraft = () => {
    const p = a.pluginDraft.trim();
    patch({
      plugins: p && !a.plugins.includes(p) ? [...a.plugins, p] : a.plugins,
      pluginDraft: "",
    });
  };
  return (
    <div className="space-y-2">
      <Label htmlFor={`plugin-${a.id}`}>plugins</Label>
      <div className="flex flex-wrap items-center gap-1">
        {a.plugins.map((p) => (
          <Badge key={p} variant="secondary" className="gap-1">
            {p}
            <button
              type="button"
              aria-label={`remove ${p}`}
              onClick={() => patch({ plugins: a.plugins.filter((x) => x !== p) })}
            >
              ×
            </button>
          </Badge>
        ))}
      </div>
      <div className="flex items-center gap-2">
        <Input
          id={`plugin-${a.id}`}
          value={a.pluginDraft}
          onChange={(e) => patch({ pluginDraft: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addDraft();
            }
          }}
          placeholder="plugin name"
          className="h-8 w-48"
        />
        <Button variant="outline" size="sm" type="button" onClick={addDraft}>
          Add plugin
        </Button>
      </div>
    </div>
  );
}

function TextField({
  field,
  label,
  placeholder,
  ariaLabel,
  agent: a,
  onPatch: patch,
}: {
  field: "harness" | "model" | "timeout";
  label: string;
  placeholder: string;
  ariaLabel?: string;
  agent: AgentDraft;
  onPatch: Patch;
}) {
  return (
    <div className="space-y-1">
      <Label htmlFor={`${field}-${a.id}`}>{label}</Label>
      <Input
        id={`${field}-${a.id}`}
        aria-label={ariaLabel}
        value={a[field]}
        onChange={(e) => patch({ [field]: e.target.value })}
        placeholder={placeholder}
        className="h-8"
      />
    </div>
  );
}

export function AgentAdvancedFields({
  agent: a,
  index: i,
  allowsPlugins,
  onPatch: patch,
}: RowProps & { allowsPlugins: boolean }) {
  return (
    <Collapsible>
      <CollapsibleTrigger asChild>
        <Button variant="outline" size="sm" type="button">
          Advanced ▾
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-3 space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            field="harness"
            label="harness"
            placeholder="image default"
            agent={a}
            onPatch={patch}
          />
          <TextField
            field="model"
            label="model"
            placeholder="harness default"
            agent={a}
            onPatch={patch}
          />
          <TextField
            field="timeout"
            label="timeout"
            ariaLabel={`timeout ${i}`}
            placeholder="for example 60m or 2h"
            agent={a}
            onPatch={patch}
          />
          <div className="space-y-1">
            <Label htmlFor={`effort-${a.id}`}>effort</Label>
            <Select value={a.effort} onValueChange={(v) => patch({ effort: v })}>
              <SelectTrigger id={`effort-${a.id}`} className="h-8">
                <SelectValue placeholder="default" />
              </SelectTrigger>
              <SelectContent>
                {EFFORT_PRESETS.map((p) => (
                  <SelectItem key={p} value={p}>
                    {p}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Switch
            id={`interactive-${a.id}`}
            checked={a.interactive}
            onCheckedChange={(v) => patch({ interactive: v })}
          />
          <Label htmlFor={`interactive-${a.id}`}>interactive (tmux TUI)</Label>
        </div>

        <AgentEnvRows agent={a} index={i} onPatch={patch} />

        {allowsPlugins && <AgentPluginsField agent={a} onPatch={patch} />}
      </CollapsibleContent>
    </Collapsible>
  );
}
