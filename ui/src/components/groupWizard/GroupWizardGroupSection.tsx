import { PathAutocomplete } from "@/components/PathAutocomplete";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

interface Props {
  name: string;
  onNameChange: (name: string) => void;
  baseCwd: string;
  onBaseCwdChange: (cwd: string) => void;
  leaderName: string;
}

export function GroupWizardGroupSection({
  name,
  onNameChange,
  baseCwd,
  onBaseCwdChange,
  leaderName,
}: Props) {
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1">
          <Label htmlFor="grp-name">group name *</Label>
          <Input
            id="grp-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            placeholder="group name"
            className="h-8"
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="grp-cwd">base cwd</Label>
          <PathAutocomplete
            id="grp-cwd"
            value={baseCwd}
            onChange={onBaseCwdChange}
            placeholder="default for every agent"
            aria-label="base cwd"
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        lead: <span className="font-mono">{leaderName || "—"}</span> (the agent marked leader below)
      </p>
      {leaderName === "" && (
        <p className="text-xs text-destructive">
          the leader row needs an explicit name so the group is created with a lead
        </p>
      )}
    </>
  );
}
