export const WORKSPACE_STATE_KEY = "terminals:workspace:v1";
export const WORKSPACE_SCHEMA_VERSION = 1 as const;
export const MAX_WORKSPACE_STATE_BYTES = 256 * 1024;
export const LEGACY_SIDEBAR_WIDTH_KEY = "terminals:sidebarWidth";

export const DEFAULT_SIDEBAR_WIDTH = 256;
export const MIN_SIDEBAR_WIDTH = 160;
export const MAX_SIDEBAR_WIDTH = 640;

const MAX_LAYOUT_WEIGHT = 1_000_000;
const TERMINAL_COMPONENT = "agent-terminal";

export interface TerminalIdentity {
  hostId: string;
  agentName: string;
}

export interface TerminalWorkspaceStateV1 {
  schemaVersion: typeof WORKSPACE_SCHEMA_VERSION;
  layout: Record<string, unknown>;
  activeTerminal: string | null;
  sidebar: {
    width: number;
    hidden: boolean;
  };
}

type JsonObject = Record<string, unknown>;

export function clampSidebarWidth(px: number): number {
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, Math.round(px)));
}

export function terminalKey(identity: TerminalIdentity): string {
  return JSON.stringify([identity.hostId, identity.agentName]);
}

export function emptyWorkspaceState(width = DEFAULT_SIDEBAR_WIDTH): TerminalWorkspaceStateV1 {
  return {
    schemaVersion: WORKSPACE_SCHEMA_VERSION,
    layout: {
      global: {},
      borders: [],
      layout: { type: "row", weight: 100, children: [] },
    },
    activeTerminal: null,
    sidebar: { width: clampSidebarWidth(width), hidden: false },
  };
}

function isObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function optionalString(value: unknown, maxLength = 256): string | undefined {
  return typeof value === "string" && value.length > 0 && value.length <= maxLength
    ? value
    : undefined;
}

function safeLayoutNumber(value: unknown): number | undefined | null {
  if (value === undefined) return undefined;
  if (
    typeof value !== "number"
    || !Number.isFinite(value)
    || value <= 0
    || value > MAX_LAYOUT_WEIGHT
  ) return null;
  return value;
}

function copyGeometry(source: JsonObject, target: JsonObject): boolean {
  for (const key of ["weight", "width", "height"] as const) {
    const value = safeLayoutNumber(source[key]);
    if (value === null) return false;
    if (value !== undefined) target[key] = value;
  }
  return true;
}

function sanitizeTerminalIdentity(value: unknown): TerminalIdentity | null {
  if (!isObject(value)) return null;
  if (typeof value.hostId !== "string" || value.hostId.length > 1024) return null;
  if (
    typeof value.agentName !== "string"
    || value.agentName.trim() === ""
    || value.agentName.length > 1024
  ) return null;
  return { hostId: value.hostId, agentName: value.agentName };
}

function sanitizeLayoutNode(
  value: unknown,
  identities: Set<string>,
  nodeIds: Set<string>,
  parentType: "row" | "tabset" | null,
): JsonObject | null {
  if (!isObject(value)) return null;
  const type = value.type;

  if (type === "tab") {
    if (parentType !== "tabset") return null;
    if (value.component !== TERMINAL_COMPONENT) return null;
    const identity = sanitizeTerminalIdentity(value.config);
    if (!identity) return null;
    const key = terminalKey(identity);
    if (identities.has(key)) return null;
    identities.add(key);

    const id = optionalString(value.id);
    if (!id || nodeIds.has(id)) return null;
    nodeIds.add(id);
    return {
      type: "tab",
      id,
      name: identity.agentName,
      component: TERMINAL_COMPONENT,
      config: identity,
    };
  }

  if (type !== "row" && type !== "tabset") return null;
  if (type === "row" && parentType === "tabset") return null;
  if (type === "tabset" && parentType !== "row") return null;
  if (!Array.isArray(value.children)) return null;
  const children: JsonObject[] = [];
  for (const child of value.children) {
    const safe = sanitizeLayoutNode(child, identities, nodeIds, type);
    if (!safe) return null;
    children.push(safe);
  }
  if (type === "row" && parentType !== null && children.length === 0) return null;
  if (type === "tabset") {
    if (children.length !== 1 || children[0].type !== "tab") return null;
  }

  const target: JsonObject = { type, children };
  if (!copyGeometry(value, target)) return null;
  const id = value.id === undefined ? undefined : optionalString(value.id);
  if (value.id !== undefined && !id) return null;
  if (id) {
    if (nodeIds.has(id)) return null;
    nodeIds.add(id);
    target.id = id;
  }
  if (type === "tabset") {
    target.selected = 0;
  }
  return target;
}

function legacySidebarWidth(): number {
  try {
    const raw = localStorage.getItem(LEGACY_SIDEBAR_WIDTH_KEY);
    if (raw === null) return DEFAULT_SIDEBAR_WIDTH;
    const parsed = Number(raw);
    return Number.isFinite(parsed) ? clampSidebarWidth(parsed) : DEFAULT_SIDEBAR_WIDTH;
  } catch {
    return DEFAULT_SIDEBAR_WIDTH;
  }
}

export function sanitizeWorkspaceState(value: unknown): TerminalWorkspaceStateV1 | null {
  if (!isObject(value) || value.schemaVersion !== WORKSPACE_SCHEMA_VERSION) return null;
  if (!isObject(value.layout) || !isObject(value.layout.layout)) return null;
  if (!Array.isArray(value.layout.borders) || value.layout.borders.length !== 0) return null;
  const identities = new Set<string>();
  const nodeIds = new Set<string>();
  const layout = sanitizeLayoutNode(value.layout.layout, identities, nodeIds, null);
  if (!layout || layout.type !== "row") return null;
  if (!isObject(value.sidebar) || typeof value.sidebar.hidden !== "boolean") return null;
  if (typeof value.sidebar.width !== "number" || !Number.isFinite(value.sidebar.width)) return null;

  let activeTerminal: string | null = null;
  if (value.activeTerminal !== null) {
    if (typeof value.activeTerminal !== "string") return null;
    if (identities.has(value.activeTerminal)) activeTerminal = value.activeTerminal;
  }

  return {
    schemaVersion: WORKSPACE_SCHEMA_VERSION,
    layout: { global: {}, borders: [], layout },
    activeTerminal,
    sidebar: {
      width: clampSidebarWidth(value.sidebar.width),
      hidden: value.sidebar.hidden,
    },
  };
}

export function readWorkspaceState(): TerminalWorkspaceStateV1 {
  try {
    const raw = localStorage.getItem(WORKSPACE_STATE_KEY);
    if (raw === null) return emptyWorkspaceState(legacySidebarWidth());
    if (new TextEncoder().encode(raw).byteLength > MAX_WORKSPACE_STATE_BYTES) {
      return emptyWorkspaceState();
    }
    return sanitizeWorkspaceState(JSON.parse(raw)) ?? emptyWorkspaceState();
  } catch {
    return emptyWorkspaceState();
  }
}

export function writeWorkspaceState(value: TerminalWorkspaceStateV1): boolean {
  const safe = sanitizeWorkspaceState(value);
  if (!safe) return false;
  const serialized = JSON.stringify(safe);
  if (new TextEncoder().encode(serialized).byteLength > MAX_WORKSPACE_STATE_BYTES) {
    return false;
  }
  try {
    localStorage.setItem(WORKSPACE_STATE_KEY, serialized);
    return true;
  } catch {
    return false;
  }
}

export function updateWorkspaceState(
  update: (current: TerminalWorkspaceStateV1) => TerminalWorkspaceStateV1,
): TerminalWorkspaceStateV1 {
  const next = sanitizeWorkspaceState(update(readWorkspaceState())) ?? emptyWorkspaceState();
  writeWorkspaceState(next);
  return next;
}
