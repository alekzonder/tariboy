import { describe, expect, it } from "vitest";
import {
  crossedDragThreshold,
  dropTargetForPoint,
} from "./workspacePointerDrag";

const rect = { left: 100, top: 50, width: 400, height: 300 };

describe("workspace pointer drag geometry", () => {
  it("waits for a deliberate pointer movement before starting", () => {
    expect(crossedDragThreshold({ x: 10, y: 10 }, { x: 13, y: 13 })).toBe(false);
    expect(crossedDragThreshold({ x: 10, y: 10 }, { x: 15, y: 10 })).toBe(true);
    expect(crossedDragThreshold({ x: 10, y: 10 }, { x: 10, y: 5 })).toBe(true);
  });

  it("selects every nearest pane edge and returns its half-pane preview", () => {
    expect(dropTargetForPoint("alpha-node", rect, 110, 200)).toEqual({
      nodeId: "alpha-node",
      dock: "left",
      preview: { left: 100, top: 50, width: 200, height: 300 },
    });
    expect(dropTargetForPoint("alpha-node", rect, 490, 200)).toEqual({
      nodeId: "alpha-node",
      dock: "right",
      preview: { left: 300, top: 50, width: 200, height: 300 },
    });
    expect(dropTargetForPoint("alpha-node", rect, 300, 55)).toEqual({
      nodeId: "alpha-node",
      dock: "top",
      preview: { left: 100, top: 50, width: 400, height: 150 },
    });
    expect(dropTargetForPoint("alpha-node", rect, 300, 345)).toEqual({
      nodeId: "alpha-node",
      dock: "bottom",
      preview: { left: 100, top: 200, width: 400, height: 150 },
    });
  });

  it("uses a stable left-first tie break at the exact center", () => {
    const square = { left: 100, top: 50, width: 300, height: 300 };
    expect(dropTargetForPoint("alpha-node", square, 250, 200).dock).toBe("left");
  });

  it("compares edge distance relative to each pane dimension", () => {
    const wide = { left: 0, top: 0, width: 1000, height: 200 };

    expect(dropTargetForPoint("wide-node", wide, 250, 100).dock).toBe("left");
    expect(dropTargetForPoint("wide-node", wide, 750, 100).dock).toBe("right");
  });
});
