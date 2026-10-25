import { useCallback, useEffect, useState } from "react";
import type { GroupRow, TeamDetail } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { GroupWizard } from "@/components/GroupWizard";
import { Button } from "@/components/ui/button";
import { useSearchParams } from "react-router-dom";
import { GroupCreateCard } from "./groups/GroupCreateCard";
import { GroupList } from "./groups/GroupList";
import { GroupDetailCard } from "./groups/GroupDetailCard";
import { TeamImportCard } from "./groups/TeamImportCard";
import { useTeamImport } from "./groups/useTeamImport";

export default function GroupsPage() {
  const [searchParams] = useSearchParams();
  const api = useApi();
  const [groups, setGroups] = useState<GroupRow[]>([]);
  const [sel, setSel] = useState<TeamDetail | null>(null);
  const [newName, setNewName] = useState("");
  const [newLead, setNewLead] = useState("");
  const [assignAgent, setAssignAgent] = useState("");
  const [wizard, setWizard] = useState(false);
  const [rename, setRename] = useState("");

  const load = useCallback(
    () =>
      api
        .request<{ groups: GroupRow[] }>("GET", "/api/groups")
        .then((r) => setGroups(r.groups))
        .catch(() => setGroups([])),
    [api],
  );
  const inspect = useCallback(
    (name: string) =>
      api
        .request<TeamDetail>("GET", `/api/groups/${encodeURIComponent(name)}`)
        .then((group) => {
          setSel(group);
          setRename(group.name);
        })
        .catch(() => setSel(null)),
    [api],
  );
  const refreshSelected = (team = sel?.name) => {
    if (team) void inspect(team);
    void load();
  };
  // The team named in the URL opens on arrival and whenever the link names another one.
  const requestedTeam = searchParams.get("team");
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (requestedTeam) void inspect(requestedTeam);
  }, [inspect, requestedTeam]);
  const teamImport = useTeamImport(api, load);

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Groups</h1>
        <Button
          size="sm"
          variant={wizard ? "secondary" : "default"}
          onClick={() => setWizard((w) => !w)}
        >
          {wizard ? "Close wizard" : "New group (wizard)"}
        </Button>
      </div>

      {wizard ? (
        <GroupWizard onCreated={() => void load()} />
      ) : (
        <GroupCreateCard
          api={api}
          name={newName}
          onNameChange={setNewName}
          lead={newLead}
          onLeadChange={setNewLead}
          onCreated={() => void load()}
        />
      )}

      <div className="grid gap-4 md:grid-cols-2">
        <GroupList groups={groups} onInspect={(name) => void inspect(name)} />
        {sel && (
          <GroupDetailCard
            api={api}
            group={sel}
            rename={rename}
            onRenameChange={setRename}
            assignAgent={assignAgent}
            onAssignAgentChange={setAssignAgent}
            onClearSelection={() => setSel(null)}
            inspect={inspect}
            load={load}
            refreshSelected={() => refreshSelected()}
          />
        )}
      </div>
      <TeamImportCard teamImport={teamImport} />
    </div>
  );
}
