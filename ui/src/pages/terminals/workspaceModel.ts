import {
  Actions,
  DockLocation,
  Model,
  TabNode,
  TabSetNode,
  type IJsonModel,
  type IJsonTabNode,
} from "flexlayout-react";
import {
  WORKSPACE_SCHEMA_VERSION,
  emptyWorkspaceState,
  sanitizeWorkspaceState,
  terminalKey,
  type TerminalIdentity,
} from "./workspaceState";

export const TERMINAL_COMPONENT = "agent-terminal";

export type TerminalDock = "left" | "right" | "top" | "bottom";

export interface AddTerminalOptions {
  relativeTo?: string;
  dock?: TerminalDock;
}

export interface AddTerminalResult {
  nodeId: string;
  added: boolean;
}

export type MoveTerminalOutcome = "moved" | "unchanged" | "restored";

export interface MoveTerminalResult {
  /**
   * "moved" — the terminal was relocated.
   * "unchanged" — the move was refused before anything changed.
   * "restored" — the move mutated the layout, was refused afterwards, and the
   *   damage was undone by rebuilding the model.
   */
  outcome: MoveTerminalOutcome;
  /**
   * The model to use from here on: the same instance unless it was restored.
   * After "restored" the previous model must be dropped, not merely ignored —
   * rebuilding adopts its tabs' view state (flexlayout's adoptViewState takes
   * moveableElement away from the old nodes), so rendering the old instance
   * afterwards shows empty panes rather than the layout as it was.
   */
  model: Model;
}

const GLOBAL_LAYOUT_OPTIONS = {
  splitterSize: 6,
  tabEnableClose: false,
  tabEnableDrag: false,
  tabEnableRename: false,
  tabEnablePopout: false,
  tabEnableRenderOnDemand: false,
  // FlexLayout only tidies an empty tabset when it is both deletable and
  // closeable. Keep the internal capability enabled while suppressing every
  // built-in close control; the app's tile close action remains detach-only.
  tabSetEnableClose: true,
  tabSetEnableCloseButton: false,
  tabSetEnableDeleteWhenEmpty: true,
  tabSetEnableMaximize: false,
  tabSetEnableTabStrip: true,
  tabSetMinWidth: 220,
  tabSetMinHeight: 140,
};

function identityFromNode(node: TabNode): TerminalIdentity | null {
  if (node.getComponent() !== TERMINAL_COMPONENT) return null;
  const config = node.getConfig() as Partial<TerminalIdentity> | undefined;
  if (
    !config
    || typeof config.hostId !== "string"
    || typeof config.agentName !== "string"
    || config.agentName.trim() === ""
  ) return null;
  return { hostId: config.hostId, agentName: config.agentName };
}

export function terminalTabJson(identity: TerminalIdentity): IJsonTabNode {
  return {
    type: "tab",
    name: identity.agentName,
    component: TERMINAL_COMPONENT,
    config: { ...identity },
    enableClose: false,
    enableDrag: false,
    enableRename: false,
    enablePopout: false,
    enableRenderOnDemand: false,
  };
}

function dockLocation(dock: TerminalDock): DockLocation {
  if (dock === "left") return DockLocation.LEFT;
  if (dock === "top") return DockLocation.TOP;
  if (dock === "bottom") return DockLocation.BOTTOM;
  return DockLocation.RIGHT;
}

// `previous` is handed to Model.fromJson so the rebuilt model adopts the live
// tabs' view state instead of remounting them: a rollback must not blink the
// running xterm sessions it is restoring.
export function createWorkspaceModel(
  layout?: Record<string, unknown>,
  previous?: Model,
): Model {
  const source = layout ?? emptyWorkspaceState().layout;
  const json = structuredClone(source) as unknown as IJsonModel;
  json.global = { ...GLOBAL_LAYOUT_OPTIONS, ...(json.global ?? {}) };
  const model = Model.fromJson(json, previous);
  model.setOnCreateTabSet(() => ({
    enableClose: true,
    enableCloseButton: false,
    enableDeleteWhenEmpty: true,
    enableMaximize: false,
    enableTabStrip: true,
    minWidth: 220,
    minHeight: 140,
  }));
  return model;
}

export function terminalIdentities(model: Model): TerminalIdentity[] {
  const identities: TerminalIdentity[] = [];
  model.visitNodes((node) => {
    if (!(node instanceof TabNode)) return;
    const identity = identityFromNode(node);
    if (identity) identities.push(identity);
  });
  return identities;
}

export function findTerminalNode(
  model: Model,
  identity: TerminalIdentity,
): TabNode | undefined {
  const wanted = terminalKey(identity);
  let found: TabNode | undefined;
  model.visitNodes((node) => {
    if (found || !(node instanceof TabNode)) return;
    const candidate = identityFromNode(node);
    if (candidate && terminalKey(candidate) === wanted) found = node;
  });
  return found;
}

function targetTabSet(model: Model, relativeTo?: string): TabSetNode | undefined {
  const relative = relativeTo ? model.getNodeById(relativeTo) : undefined;
  if (relative instanceof TabSetNode) return relative;
  if (relative instanceof TabNode && relative.getParent() instanceof TabSetNode) {
    return relative.getParent() as TabSetNode;
  }
  const active = model.getActiveTabset();
  if (active) return active;
  const existing = terminalIdentities(model).at(-1);
  if (existing) {
    const node = findTerminalNode(model, existing);
    if (node?.getParent() instanceof TabSetNode) return node.getParent() as TabSetNode;
  }
  return model.getFirstTabSet();
}

export function addTerminalToModel(
  model: Model,
  identity: TerminalIdentity,
  options: AddTerminalOptions = {},
): AddTerminalResult {
  const existing = findTerminalNode(model, identity);
  if (existing) {
    model.doAction(Actions.selectTab(existing.getId()));
    return { nodeId: existing.getId(), added: false };
  }

  const target = targetTabSet(model, options.relativeTo);
  if (!target) throw new Error("workspace has no layout target");
  const hasTerminals = terminalIdentities(model).length > 0;
  const location = hasTerminals
    ? dockLocation(options.dock ?? "right")
    : DockLocation.CENTER;
  const added = model.doAction(
    Actions.addTab(terminalTabJson(identity), target.getId(), location, -1, true),
  );
  if (!(added instanceof TabNode)) throw new Error("terminal layout add failed");
  return { nodeId: added.getId(), added: true };
}

export function replaceTerminalInModel(
  model: Model,
  nodeId: string,
  identity: TerminalIdentity,
): boolean {
  const node = model.getNodeById(nodeId);
  if (!(node instanceof TabNode)) return false;
  const duplicate = findTerminalNode(model, identity);
  if (duplicate && duplicate.getId() !== nodeId) return false;
  model.doAction(Actions.updateNodeAttributes(nodeId, {
    name: identity.agentName,
    config: { ...identity },
  }));
  model.doAction(Actions.selectTab(nodeId));
  return true;
}

export function removeTerminalFromModel(model: Model, nodeId: string): boolean {
  if (!(model.getNodeById(nodeId) instanceof TabNode)) return false;
  model.doAction(Actions.deleteTab(nodeId));
  return true;
}

export function moveTerminalInModel(
  model: Model,
  nodeId: string,
  relativeTo: string,
  dock: TerminalDock,
): MoveTerminalResult {
  // "unchanged" and "restored" both mean the move was rejected, but they are
  // not interchangeable: "unchanged" promises the model was never touched,
  // "restored" says it was mutated and the replacement below is the layout the
  // caller must switch to.
  const unchanged: MoveTerminalResult = { outcome: "unchanged", model };
  if (nodeId === relativeTo) return unchanged;
  const source = model.getNodeById(nodeId);
  const target = model.getNodeById(relativeTo);
  if (!(source instanceof TabNode) || !(target instanceof TabNode)) return unchanged;
  if (!identityFromNode(source) || !identityFromNode(target)) return unchanged;

  // FlexLayout only drops onto a tabset, border or row: MOVE_NODE with a tab id
  // as its destination matches none of them and is silently ignored. Resolve the
  // drop target to the tabset that owns it, the way addTerminalToModel already
  // does. targetTabSet falls back to the active tabset when the node has no
  // tabset parent, which for a move would relocate the terminal somewhere the
  // user never pointed at — so require the resolved tabset to actually own the
  // target tab.
  const targetSet = targetTabSet(model, target.getId());
  if (!targetSet || !targetSet.getChildren().some((child) => child === target)) return unchanged;

  const before = terminalIdentities(model).length;
  const layoutBefore = JSON.stringify(model.toJson());
  model.doAction(Actions.moveNode(
    source.getId(),
    targetSet.getId(),
    dockLocation(dock),
    -1,
    true,
  ));
  // Both rejections below fire after doAction has already changed the layout the
  // workspace is rendering. flexlayout's Model cannot be restored in place — the
  // only way back from json is the static Model.fromJson, which builds a new
  // object — so the rollback is a replacement model the caller has to adopt.
  const restored = (): MoveTerminalResult => ({
    outcome: "restored",
    model: createWorkspaceModel(JSON.parse(layoutBefore) as Record<string, unknown>, model),
  });
  if (terminalIdentities(model).length !== before) return restored();
  // A move that leaves the layout byte-identical did not happen. Without this
  // the invariant check below reports success for a no-op, which is exactly how
  // the failed drag stayed silent.
  if (JSON.stringify(model.toJson()) === layoutBefore) return unchanged;

  let valid = true;
  model.visitNodes((node) => {
    if (
      node instanceof TabSetNode
      && node.getChildren().some((child) => child instanceof TabNode && identityFromNode(child))
      && node.getChildren().length !== 1
    ) {
      valid = false;
    }
  });
  return valid ? { outcome: "moved", model } : restored();
}

export function workspaceJson(model: Model): Record<string, unknown> {
  if (terminalIdentities(model).length === 0) return emptyWorkspaceState().layout;
  const safe = sanitizeWorkspaceState({
    schemaVersion: WORKSPACE_SCHEMA_VERSION,
    layout: model.toJson(),
    activeTerminal: null,
    sidebar: { width: 256, hidden: false },
  });
  if (!safe) throw new Error("terminal layout failed validation");
  return safe.layout;
}
