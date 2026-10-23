import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { CreateAgentForm } from "./useCreateAgentForm";

export function PluginsFields({
  draft,
  updateDraft,
  formDisabled,
  displayedPlugins,
  schemaV2,
  pluginDraft,
  setPluginDraft,
  addPlugin,
}: Pick<
  CreateAgentForm,
  | "draft"
  | "updateDraft"
  | "formDisabled"
  | "displayedPlugins"
  | "schemaV2"
  | "pluginDraft"
  | "setPluginDraft"
  | "addPlugin"
>) {
  return (
    <div className="space-y-2">
      <Label>plugins</Label>
      <div className="flex flex-wrap gap-1">
        {displayedPlugins.length === 0 && (
          <span className="text-xs text-muted-foreground">none</span>
        )}
        {displayedPlugins.map((plugin) => (
          <Badge key={plugin} variant="secondary" className="gap-1">
            {plugin}
            {!schemaV2 && (
              <button
                type="button"
                aria-label={`remove ${plugin}`}
                onClick={() =>
                  updateDraft(
                    "plugins",
                    draft.plugins.filter((value) => value !== plugin),
                  )
                }
              >
                ×
              </button>
            )}
          </Badge>
        ))}
      </div>
      {schemaV2 ? (
        <p className="text-xs text-muted-foreground">
          Plugins are owned by this schema-v2 image.
        </p>
      ) : (
        <div className="flex gap-2">
          <Input
            aria-label="plugin name"
            value={pluginDraft}
            onChange={(event) => setPluginDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                addPlugin();
              }
            }}
            disabled={formDisabled}
            className="h-8"
          />
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={addPlugin}
            disabled={formDisabled}
          >
            Add plugin
          </Button>
        </div>
      )}
    </div>
  );
}
