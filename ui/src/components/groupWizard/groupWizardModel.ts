import type { CreateAgentSpec, ImageSummary } from "@/lib/api";
import { serializeEnv, type EnvRow } from "@/components/agentFormValues";

// One draft agent row in the wizard. `id` is a stable key so per-row result
// status survives add/remove churn. `override` reveals the row's own cwd
// PathAutocomplete; when off, the row inherits the group's base cwd.
export interface AgentDraft {
  id: number;
  image: string;
  name: string;
  override: boolean;
  cwd: string;
  model: string;
  effort: string;
  harness: string;
  timeout: string;
  interactive: boolean;
  envRows: EnvRow[];
  plugins: string[];
  pluginDraft: string;
}

// Per-row orchestration outcome, keyed by draft id.
export type RowStatus =
  | { kind: "creating" }
  | { kind: "created"; name: string }
  | { kind: "failed"; reason: string };

export function emptyDraft(id: number): AgentDraft {
  return {
    id,
    image: "",
    name: "",
    override: false,
    cwd: "",
    model: "",
    effort: "",
    harness: "",
    timeout: "",
    interactive: false,
    envRows: [],
    plugins: [],
    pluginDraft: "",
  };
}

export interface WizardState {
  agents: AgentDraft[];
  nextId: number;
  leaderIdx: number;
  rowStatus: Record<number, RowStatus>;
}

export type WizardAction =
  | { type: "addAgent" }
  | { type: "removeAgent"; id: number; idx: number }
  | { type: "patchAgent"; id: number; patch: Partial<AgentDraft> }
  | { type: "setLeader"; idx: number }
  | { type: "setRowStatus"; id: number; status: RowStatus };

export function initialWizardState(): WizardState {
  return { agents: [emptyDraft(0)], nextId: 1, leaderIdx: 0, rowStatus: {} };
}

export function groupWizardReducer(state: WizardState, action: WizardAction): WizardState {
  switch (action.type) {
    case "addAgent":
      return {
        ...state,
        agents: [...state.agents, emptyDraft(state.nextId)],
        nextId: state.nextId + 1,
      };
    case "removeAgent": {
      const { idx } = action;
      const li = state.leaderIdx;
      return {
        ...state,
        agents: state.agents.filter((r) => r.id !== action.id),
        // Keep the leader marker pointing at a valid row.
        leaderIdx: idx < li ? li - 1 : idx === li ? 0 : li,
      };
    }
    case "patchAgent":
      return {
        ...state,
        agents: state.agents.map((r) => (r.id === action.id ? { ...r, ...action.patch } : r)),
      };
    case "setLeader":
      return { ...state, leaderIdx: action.idx };
    case "setRowStatus":
      return { ...state, rowStatus: { ...state.rowStatus, [action.id]: action.status } };
  }
}

export function leaderNameOf(agents: AgentDraft[], leaderIdx: number): string {
  return agents[leaderIdx]?.name.trim() || "";
}

// Submit is gated on a group name, every row having an image, AND the leader
// row having an explicit name. Without the last gate a blank leader name makes
// lead=undefined at POST /api/groups, creating the group leaderless — the
// intended leader then joins as a plain member via its group field.
export function canCreate(name: string, agents: AgentDraft[], leaderIdx: number): boolean {
  return (
    name.trim() !== "" &&
    leaderNameOf(agents, leaderIdx) !== "" &&
    agents.length > 0 &&
    agents.every((a) => a.image)
  );
}

export function allowsPluginOverrides(images: ImageSummary[], ref: string): boolean {
  const selected = images.find((candidate) => `${candidate.name}:${candidate.tag}` === ref);
  return selected?.schema_version === 1;
}

export function specFor(
  a: AgentDraft,
  baseCwd: string,
  group: string,
  allowsPlugins: boolean,
): CreateAgentSpec {
  return {
    image: a.image,
    name: a.name.trim() || undefined,
    cwd: (a.override ? a.cwd.trim() : baseCwd.trim()) || undefined,
    model: a.model.trim() || undefined,
    effort: a.effort || undefined,
    harness: a.harness.trim() || undefined,
    interactive: a.interactive,
    env: serializeEnv(a.envRows) || undefined,
    plugins: allowsPlugins ? a.plugins.join(",") || undefined : undefined,
    timeout: a.timeout.trim() || undefined,
    group: group.trim(),
  };
}
