import type { ApiClient, GroupView } from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

interface Props {
  api: ApiClient;
  group: GroupView;
  rename: string;
  onRenameChange: (name: string) => void;
  assignAgent: string;
  onAssignAgentChange: (agent: string) => void;
  onClearSelection: () => void;
  inspect: (name: string) => Promise<void>;
  load: () => Promise<void>;
  refreshSelected: () => void;
}

const saveBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
};

function RemoveGroupDialog({ api, group: sel, onClearSelection, load }: Props) {
  const remove = () =>
    guard("remove", () => api.request("DELETE", `/api/groups/${encodeURIComponent(sel.name)}`), {
      verb: "ok",
      after: () => {
        onClearSelection();
        void load();
      },
    });
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant="destructive">
          Remove
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove group {sel.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            Detaches members and deletes the group's channels.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={remove}>Remove</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function GroupMembers({ api, group: sel, refreshSelected }: Props) {
  const setLead = (lead: string) =>
    guard("lead", () => api.groups.setTeamLead(sel.name, lead), {
      verb: "ok",
      after: () => refreshSelected(),
    });
  const removeMember = (member: string) =>
    guard("remove member", () => api.groups.removeTeamMember(sel.name, member), {
      verb: "ok",
      after: () => refreshSelected(),
    });
  return (
    <>
      <div className="flex items-center gap-2">
        lead:
        <select
          aria-label="team lead"
          value={sel.lead}
          className="h-8 rounded border bg-background px-2"
          onChange={(event) => setLead(event.target.value)}
        >
          <option value="">No lead</option>
          {sel.members.map((member) => (
            <option key={member} value={member}>
              {member}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-1">
        members:
        {sel.members.map((member) => (
          <div key={member} className="flex items-center justify-between rounded border px-2 py-1">
            <span className="font-mono">{member}</span>
            <Button
              size="sm"
              variant="ghost"
              aria-label={`Remove ${member}`}
              onClick={() => removeMember(member)}
            >
              Remove
            </Button>
          </div>
        ))}
      </div>
    </>
  );
}

function GroupExportActions({ api, group: sel }: Pick<Props, "api" | "group">) {
  const copyYaml = () =>
    void api.groups
      .getTeamCompose(sel.name)
      .then((result) => navigator.clipboard.writeText(result.yaml))
      .then(() => toast.success("compose YAML copied"))
      .catch((error) => toast.error(String(error)));
  const exportArchive = () =>
    void api.groups
      .downloadTeamArchive(sel.name)
      .then((blob) => saveBlob(blob, `${sel.name}.tar.gz`))
      .catch((error) => toast.error(String(error)));
  return (
    <div className="flex flex-wrap gap-2 border-t pt-3">
      <Button size="sm" variant="outline" aria-label="Copy compose YAML" onClick={copyYaml}>
        Copy YAML
      </Button>
      <Button size="sm" variant="outline" aria-label="Export team archive" onClick={exportArchive}>
        Export archive
      </Button>
    </div>
  );
}

export function GroupDetailCard(props: Props) {
  const { api, group: sel, rename, assignAgent, onClearSelection, inspect, load } = props;
  const renameTeam = () =>
    guard("rename", () => api.groups.renameTeam(sel.name, rename), {
      verb: "ok",
      after: () => {
        onClearSelection();
        void inspect(rename);
        void load();
      },
    });
  const assign = () =>
    guard(
      "assign",
      () =>
        api.request("POST", `/api/groups/${encodeURIComponent(sel.name)}/assign`, {
          agent: assignAgent,
        }),
      {
        verb: "ok",
        after: () => {
          props.onAssignAgentChange("");
          void inspect(sel.name);
          void load();
        },
      },
    );
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-base font-mono">{sel.name}</CardTitle>
        <RemoveGroupDialog {...props} />
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <div className="flex gap-2">
          <Input
            aria-label="team name"
            value={rename}
            onChange={(event) => props.onRenameChange(event.target.value)}
            className="h-8"
          />
          <Button size="sm" variant="outline" onClick={renameTeam}>
            Rename
          </Button>
        </div>
        <GroupMembers {...props} />
        <div className="text-xs text-muted-foreground">shared: {sel.shared_dir}</div>
        <div className="flex items-end gap-2 pt-2">
          <Input
            value={assignAgent}
            onChange={(e) => props.onAssignAgentChange(e.target.value)}
            placeholder="agent to assign"
            className="h-8 w-48"
          />
          <Button size="sm" onClick={assign}>
            Assign
          </Button>
        </div>
        <GroupExportActions api={api} group={sel} />
      </CardContent>
    </Card>
  );
}
