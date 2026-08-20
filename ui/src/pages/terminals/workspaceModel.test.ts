import { describe, expect, it, vi } from "vitest";
import { Actions, DockLocation, Node, Orientation, TabNode, TabSetNode } from "flexlayout-react";
import {
  addTerminalToModel,
  createWorkspaceModel,
  findTerminalNode,
  moveTerminalInModel,
  removeTerminalFromModel,
  replaceTerminalInModel,
  terminalIdentities,
  terminalTabJson,
  workspaceJson,
} from "./workspaceModel";
import type { TerminalIdentity } from "./workspaceState";

const alpha: TerminalIdentity = { hostId: "", agentName: "alpha" };
const beta: TerminalIdentity = { hostId: "remote-a", agentName: "beta" };
const gamma: TerminalIdentity = { hostId: "remote-b", agentName: "gamma" };

function tabsets(model: ReturnType<typeof createWorkspaceModel>): TabSetNode[] {
  const result: TabSetNode[] = [];
  model.visitNodes((node) => {
    if (node instanceof TabSetNode) result.push(node);
  });
  return result;
}

function tabsetOf(model: ReturnType<typeof createWorkspaceModel>, nodeId: string): TabSetNode {
  const node = model.getNodeById(nodeId);
  if (!(node instanceof TabNode)) throw new Error(`no tab ${nodeId}`);
  const parent = node.getParent();
  if (!(parent instanceof TabSetNode)) throw new Error(`tab ${nodeId} has no tabset`);
  return parent;
}

// The row both tabsets sit in, with the axis it stacks them along. Ask
// FlexLayout for the orientation instead of deriving it from nesting depth:
// "nested below the root" only means vertical while the root itself is
// horizontal, so a model built with rootOrientationVertical would flip the
// real layout while keeping the derived answer — and the assertion — unchanged.
function siblingRow(a: TabSetNode, b: TabSetNode): { row: Node; orientation: Orientation; order: number[] } {
  const row = a.getParent();
  expect(row).toBeDefined();
  expect(b.getParent()).toBe(row);
  const children = row!.getChildren();
  return {
    row: row!,
    orientation: row!.getOrientation(),
    order: [children.indexOf(a), children.indexOf(b)],
  };
}

// Two terminals sharing one tabset — the invariant moveTerminalInModel checks
// for, already violated before the move, so that any accepted move keeps it
// violated and the rejection lands after the mutation.
function pairedTabsetModel() {
  return createWorkspaceModel({
    layout: {
      type: "row",
      children: [
        {
          type: "tabset",
          id: "paired",
          children: [
            { ...terminalTabJson(alpha), id: "tab-alpha" },
            { ...terminalTabJson(beta), id: "tab-beta" },
          ],
        },
        {
          type: "tabset",
          id: "solo",
          children: [{ ...terminalTabJson(gamma), id: "tab-gamma" }],
        },
      ],
    },
  });
}

describe("terminal workspace model", () => {
  it("adds a root terminal and splits a second terminal to its right", () => {
    const model = createWorkspaceModel();

    const first = addTerminalToModel(model, alpha);
    const second = addTerminalToModel(model, beta, {
      relativeTo: first.nodeId,
      dock: "right",
    });

    expect(first.added).toBe(true);
    expect(second.added).toBe(true);
    expect(terminalIdentities(model)).toEqual([alpha, beta]);
    const json = workspaceJson(model);
    const root = json.layout as { children: Array<{ children: unknown[] }> };
    expect(root.children).toHaveLength(2);
    expect(root.children.every((child) => child.children.length === 1)).toBe(true);
  });

  it("focuses the existing node instead of adding a duplicate", () => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);

    const duplicate = addTerminalToModel(model, alpha);

    expect(duplicate).toEqual({ nodeId: first.nodeId, added: false });
    expect(terminalIdentities(model)).toEqual([alpha]);
  });

  it("replaces an identity without changing the node or tree position", () => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);
    addTerminalToModel(model, beta, { relativeTo: first.nodeId, dock: "bottom" });
    const beforeParent = findTerminalNode(model, alpha)?.getParent()?.getId();

    expect(replaceTerminalInModel(model, first.nodeId, gamma)).toBe(true);

    expect(findTerminalNode(model, alpha)).toBeUndefined();
    expect(findTerminalNode(model, gamma)?.getId()).toBe(first.nodeId);
    expect(findTerminalNode(model, gamma)?.getParent()?.getId()).toBe(beforeParent);
  });

  it("rejects replacement with an identity already present elsewhere", () => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);
    addTerminalToModel(model, beta, { relativeTo: first.nodeId, dock: "right" });

    expect(replaceTerminalInModel(model, first.nodeId, beta)).toBe(false);
    expect(terminalIdentities(model)).toEqual([alpha, beta]);
  });

  it("removes a terminal and leaves the other split intact", () => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);
    const second = addTerminalToModel(model, beta, {
      relativeTo: first.nodeId,
      dock: "right",
    });

    expect(removeTerminalFromModel(model, second.nodeId)).toBe(true);

    expect(terminalIdentities(model)).toEqual([alpha]);
  });

  it("moves a terminal between edges without retaining an empty tabset", () => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);
    const second = addTerminalToModel(model, beta, {
      relativeTo: first.nodeId,
      dock: "right",
    });
    const firstParent = findTerminalNode(model, alpha)?.getParent();
    expect(firstParent).toBeInstanceOf(TabSetNode);

    model.doAction(Actions.moveNode(
      second.nodeId,
      firstParent!.getId(),
      DockLocation.BOTTOM,
      -1,
      true,
    ));

    const tabsets: TabSetNode[] = [];
    model.visitNodes((node) => {
      if (node instanceof TabSetNode) tabsets.push(node);
    });
    expect(tabsets).toHaveLength(2);
    expect(tabsets.every((node) => node.getChildren().length === 1)).toBe(true);
    expect(() => workspaceJson(model)).not.toThrow();
  });

  it.each(["left", "right", "top", "bottom"] as const)(
    "moves a terminal to the %s edge through the application adapter",
    (dock) => {
      const model = createWorkspaceModel();
      const first = addTerminalToModel(model, alpha);
      const second = addTerminalToModel(model, beta, {
        relativeTo: first.nodeId,
        dock: "right",
      });
      addTerminalToModel(model, gamma, {
        relativeTo: second.nodeId,
        dock: "bottom",
      });

      expect(moveTerminalInModel(model, second.nodeId, first.nodeId, dock))
        .toEqual({ outcome: "moved", model });
      expect(terminalIdentities(model)).toEqual(expect.arrayContaining([alpha, beta, gamma]));
      expect(tabsets(model)).toHaveLength(3);
      expect(tabsets(model).every((node) => node.getChildren().length === 1)).toBe(true);
      expect(() => workspaceJson(model)).not.toThrow();
    },
  );

  it.each([
    ["top", true] as const,
    ["bottom", false] as const,
  ])("stacks the dragged terminal %s of its neighbour", (dock, draggedFirst) => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);
    const second = addTerminalToModel(model, beta, {
      relativeTo: first.nodeId,
      dock: "right",
    });

    expect(moveTerminalInModel(model, second.nodeId, first.nodeId, dock))
      .toEqual({ outcome: "moved", model });

    const layout = siblingRow(tabsetOf(model, second.nodeId), tabsetOf(model, first.nodeId));
    expect(layout.orientation).toBe(Orientation.VERT);
    const [betaIndex, alphaIndex] = layout.order;
    expect(betaIndex < alphaIndex).toBe(draggedFirst);
    expect(tabsets(model)).toHaveLength(2);
    expect(tabsets(model).every((node) => node.getChildren().length === 1)).toBe(true);
    expect(terminalIdentities(model)).toEqual(expect.arrayContaining([alpha, beta]));
    expect(() => workspaceJson(model)).not.toThrow();
  });

  it.each([
    ["left", true] as const,
    ["right", false] as const,
  ])("keeps the %s dock side by side", (dock, draggedFirst) => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);
    const second = addTerminalToModel(model, beta, {
      relativeTo: first.nodeId,
      dock: "bottom",
    });

    expect(moveTerminalInModel(model, second.nodeId, first.nodeId, dock))
      .toEqual({ outcome: "moved", model });

    const layout = siblingRow(tabsetOf(model, second.nodeId), tabsetOf(model, first.nodeId));
    expect(layout.orientation).toBe(Orientation.HORZ);
    const [betaIndex, alphaIndex] = layout.order;
    expect(betaIndex < alphaIndex).toBe(draggedFirst);
    expect(tabsets(model)).toHaveLength(2);
    expect(() => workspaceJson(model)).not.toThrow();
  });

  it("reports failure when the layout action leaves the model unchanged", () => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);
    const second = addTerminalToModel(model, beta, {
      relativeTo: first.nodeId,
      dock: "right",
    });
    const beforeJson = JSON.stringify(model.toJson());

    // A layout action flexlayout silently ignores (the class of failure that hid
    // this bug: no throw, no rejection, simply nothing happens).
    const doAction = vi.spyOn(model, "doAction").mockReturnValue(undefined);
    try {
      // A rejection with no mutation must stay "unchanged": it is the caller's
      // signal that nothing happened and the model it holds is still current.
      expect(moveTerminalInModel(model, second.nodeId, first.nodeId, "top"))
        .toEqual({ outcome: "unchanged", model });
    } finally {
      doAction.mockRestore();
    }
    expect(JSON.stringify(model.toJson())).toBe(beforeJson);
  });

  it("restores the layout when the move breaks the one-terminal-per-tabset rule", () => {
    // A layout where two terminals already share a tabset. Moving a third one
    // onto that tabset is accepted by flexlayout, so the invariant check fires
    // *after* the model has been mutated — the path that used to leave the user
    // looking at a layout the code believed had never changed.
    const model = pairedTabsetModel();
    const layoutBefore = JSON.stringify(model.toJson());

    const result = moveTerminalInModel(model, "tab-gamma", "tab-alpha", "left");

    expect(result.outcome).toBe("restored");
    expect(result.model).not.toBe(model);
    // The rejected move really did mutate the model the caller was holding:
    // without the swap below the workspace would keep rendering that layout.
    expect(JSON.stringify(model.toJson())).not.toBe(layoutBefore);
    expect(JSON.stringify(result.model.toJson())).toBe(layoutBefore);
  });

  it("restores the layout when the move loses a terminal", () => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);
    const second = addTerminalToModel(model, beta, {
      relativeTo: first.nodeId,
      dock: "right",
    });
    const third = addTerminalToModel(model, gamma, {
      relativeTo: second.nodeId,
      dock: "bottom",
    });
    const layoutBefore = JSON.stringify(model.toJson());

    // flexlayout dropping a terminal while performing the move: the move itself
    // succeeds, but the workspace comes back one terminal short.
    const doAction = model.doAction.bind(model);
    const spy = vi.spyOn(model, "doAction").mockImplementation((action) => {
      const applied = doAction(action);
      doAction(Actions.deleteTab(third.nodeId));
      return applied;
    });
    let result;
    try {
      result = moveTerminalInModel(model, second.nodeId, first.nodeId, "top");
    } finally {
      spy.mockRestore();
    }

    expect(result.outcome).toBe("restored");
    expect(JSON.stringify(model.toJson())).not.toBe(layoutBefore);
    expect(JSON.stringify(result.model.toJson())).toBe(layoutBefore);
    expect(terminalIdentities(result.model)).toEqual(
      expect.arrayContaining([alpha, beta, gamma]),
    );
  });

  it("separates a harmless no-op from a rejected mutation", () => {
    const noop = createWorkspaceModel();
    const only = addTerminalToModel(noop, alpha);
    const noopJson = JSON.stringify(noop.toJson());

    // Same false as before the split, two different meanings: nothing was done…
    expect(moveTerminalInModel(noop, only.nodeId, only.nodeId, "right"))
      .toEqual({ outcome: "unchanged", model: noop });
    expect(JSON.stringify(noop.toJson())).toBe(noopJson);

    // …versus something was done and had to be undone.
    const broken = pairedTabsetModel();
    expect(moveTerminalInModel(broken, "tab-gamma", "tab-alpha", "left").outcome)
      .toBe("restored");
  });

  it("hands the rolled back model the previous model's view state", () => {
    const model = pairedTabsetModel();

    const result = moveTerminalInModel(model, "tab-gamma", "tab-alpha", "left");

    // Model.fromJson(json, previousModel) records the model it adopted from and
    // copies each matching tab's view state across, so live terminals are not
    // remounted by the rollback. getAdoptedFromModel is flexlayout-internal and
    // absent from its public typings, hence the cast.
    const adopted = (result.model as unknown as {
      getAdoptedFromModel(): unknown;
    }).getAdoptedFromModel();
    expect(adopted).toBe(model);
  });

  it("rejects missing and self-target model moves", () => {
    const model = createWorkspaceModel();
    const first = addTerminalToModel(model, alpha);

    expect(moveTerminalInModel(model, first.nodeId, first.nodeId, "left"))
      .toEqual({ outcome: "unchanged", model });
    expect(moveTerminalInModel(model, "missing", first.nodeId, "right"))
      .toEqual({ outcome: "unchanged", model });
    expect(moveTerminalInModel(model, first.nodeId, "missing", "bottom"))
      .toEqual({ outcome: "unchanged", model });
    expect(terminalIdentities(model)).toEqual([alpha]);
  });

  it("serializes an empty model as a valid empty workspace layout", () => {
    const model = createWorkspaceModel();

    expect(workspaceJson(model)).toMatchObject({
      layout: { type: "row" },
    });
    expect(terminalIdentities(model)).toEqual([]);
  });
});
