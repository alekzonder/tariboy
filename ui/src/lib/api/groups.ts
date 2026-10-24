import type { Transport } from "./client";

// One group as listed by GET /api/groups. Mirrors group.ls: name/lead plus a
// member count.
export interface GroupRow {
  name: string;
  lead: string;
  members: number;
}
export type TeamRow = GroupRow;
export interface TeamDetail {
  name: string;
  lead: string;
  members: string[];
  broadcast: string;
  inbox: string;
  shared_dir: string;
}
export interface TeamImportResult {
  groups: number;
  agents: Array<{ name: string; status: "created" | "reused" | "failed"; error?: string }>;
  complete: boolean;
  import_id?: string;
}
export interface TeamImportOperation {
  import_id: string;
  team: string;
  status: "pending" | "running" | "complete" | "failed";
  steps: Array<{ kind: "image" | "agent"; name: string; status: string; error?: string }>;
  error?: string;
  updated_at: string;
}
export interface TeamImportPreview {
  import_id: string;
  team: string;
  yaml: string;
  images: Array<{ ref: string; action: string; conflict: boolean; message?: string }>;
  agents: Array<{ name: string; action: string; conflict: boolean; message?: string }>;
  groups: Array<{ name: string; action: string; conflict: boolean; message?: string }>;
}

const groupPath = (name: string) => `/api/groups/${encodeURIComponent(name)}`;

export function groupsApi(t: Transport) {
  return {
    listGroups: () => t.get<{ groups: GroupRow[]; count: number }>("/api/groups"),
    getTeam: (name: string) => t.get<TeamDetail>(groupPath(name)),
    renameTeam: (name: string, newName: string) =>
      t.request<TeamDetail>("PATCH", `${groupPath(name)}/name`, { new_name: newName }),
    setTeamLead: (name: string, lead: string) =>
      t.request<TeamDetail>("PATCH", `${groupPath(name)}/lead`, { lead }),
    addExistingMember: (name: string, agent: string) =>
      t.post(`${groupPath(name)}/assign`, { agent }),
    removeTeamMember: (name: string, agent: string) =>
      t.delete(`${groupPath(name)}/members/${encodeURIComponent(agent)}`),
    getTeamCompose: (name: string) =>
      t.get<{ name: string; yaml: string }>(`${groupPath(name)}/compose`),
    importTeamYAML: (yaml: string) =>
      t.post<TeamImportPreview>("/api/team-imports/preview-yaml", { yaml }),
    applyTeamArchive: (
      importID: string,
      resolution?: { refs?: Record<string, string>; yaml?: string; update_existing?: boolean },
    ) =>
      t.post<TeamImportResult>(
        `/api/team-imports/${encodeURIComponent(importID)}/apply`,
        resolution ?? {},
      ),
    getTeamImportOperation: (importID: string) =>
      t.get<TeamImportOperation>(`/api/team-imports/${encodeURIComponent(importID)}`),
    downloadTeamArchive: async (name: string) =>
      (await t.raw("GET", `${groupPath(name)}/export`)).blob(),
    uploadTeamArchive: async (archive: Blob) => {
      const response = await t.raw("POST", "/api/team-imports", archive);
      const envelope = (await response.json()) as { result: TeamImportPreview };
      return {
        ...envelope.result,
        agents: envelope.result.agents ?? [],
        groups: envelope.result.groups ?? [],
      };
    },
  };
}
export type GroupsApi = ReturnType<typeof groupsApi>;
