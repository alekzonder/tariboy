import type {
  GroupList,
  GroupView,
  TeamCompose,
  TeamImportApplyResult,
  TeamImportOperation,
  TeamImportPlan,
} from "../api-types.generated";
import type { Transport } from "./client";

export type {
  GroupRow,
  GroupView,
  TeamImportApplyResult,
  TeamImportOperation,
  TeamImportPlan,
} from "../api-types.generated";

const groupPath = (name: string) => `/api/groups/${encodeURIComponent(name)}`;

export function groupsApi(t: Transport) {
  return {
    listGroups: () => t.get<GroupList>("/api/groups"),
    getTeam: (name: string) => t.get<GroupView>(groupPath(name)),
    renameTeam: (name: string, newName: string) =>
      t.request<GroupView>("PATCH", `${groupPath(name)}/name`, { new_name: newName }),
    setTeamLead: (name: string, lead: string) =>
      t.request<GroupView>("PATCH", `${groupPath(name)}/lead`, { lead }),
    addExistingMember: (name: string, agent: string) =>
      t.post(`${groupPath(name)}/assign`, { agent }),
    removeTeamMember: (name: string, agent: string) =>
      t.delete(`${groupPath(name)}/members/${encodeURIComponent(agent)}`),
    getTeamCompose: (name: string) => t.get<TeamCompose>(`${groupPath(name)}/compose`),
    importTeamYAML: (yaml: string) =>
      t.post<TeamImportPlan>("/api/team-imports/preview-yaml", { yaml }),
    applyTeamArchive: (
      importID: string,
      resolution?: { refs?: Record<string, string>; yaml?: string; update_existing?: boolean },
    ) =>
      t.post<TeamImportApplyResult>(
        `/api/team-imports/${encodeURIComponent(importID)}/apply`,
        resolution ?? {},
      ),
    getTeamImportOperation: (importID: string) =>
      t.get<TeamImportOperation>(`/api/team-imports/${encodeURIComponent(importID)}`),
    downloadTeamArchive: async (name: string) =>
      (await t.raw("GET", `${groupPath(name)}/export`)).blob(),
    uploadTeamArchive: async (archive: Blob) => {
      const response = await t.raw("POST", "/api/team-imports", archive);
      // Not in /api/openapi.json: served by internal/api/server.go POST /api/team-imports.
      const envelope = (await response.json()) as { result: TeamImportPlan };
      return {
        ...envelope.result,
        agents: envelope.result.agents ?? [],
        groups: envelope.result.groups ?? [],
      };
    },
  };
}
