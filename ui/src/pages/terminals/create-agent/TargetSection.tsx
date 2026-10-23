import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { ImageCombobox } from "@/components/AgentFormFields";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { CreateAgentForm } from "./useCreateAgentForm";

const LOCAL_HOST_VALUE = "__local__";

export function TargetSection({
  host,
  hosts,
  draft,
  formDisabled,
  changeHost,
  changeImage,
  bare,
  images,
  loadingImages,
}: Pick<
  CreateAgentForm,
  | "host"
  | "hosts"
  | "draft"
  | "formDisabled"
  | "changeHost"
  | "changeImage"
  | "bare"
  | "images"
  | "loadingImages"
>) {
  return (
    <section
      aria-labelledby="create-agent-target"
      className="space-y-3 rounded border p-3"
    >
      <h3 id="create-agent-target" className="font-medium">
        Target
      </h3>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1">
          <Label htmlFor="create-agent-host">Host</Label>
          <Select
            value={host || LOCAL_HOST_VALUE}
            onValueChange={changeHost}
            disabled={formDisabled}
          >
            <SelectTrigger id="create-agent-host" className="h-8">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {hosts.map((entry) => (
                <SelectItem
                  key={entry.id || LOCAL_HOST_VALUE}
                  value={entry.id || LOCAL_HOST_VALUE}
                >
                  {entry.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <Label htmlFor="create-agent-image">Image</Label>
            {bare && <Badge variant="secondary">Terminal only</Badge>}
          </div>
          <ImageCombobox
            id="create-agent-image"
            images={images}
            value={draft.image}
            onChange={changeImage}
            ariaLabel="image"
          />
          {loadingImages && (
            <p className="text-xs text-muted-foreground">
              Loading built images…
            </p>
          )}
        </div>
      </div>
    </section>
  );
}
