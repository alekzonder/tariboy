import { describe, expect, it } from "vitest";
import { keyIndentPx, MAX_INDENT_DEPTH } from "./taskColumns";

// Before the key on a phone: the gap after the indent, the chevron, the gap,
// the queue marker and the gap after it.
const PHONE_KEY_PREFIX_PX = 6 + 16 + 6 + 12 + 6;
// Four monospace characters and an ellipsis, about 7px each.
const FIVE_CHARS_PX = 5 * 7;
const PHONE_KEY_COLUMN_PX = 96;

describe("keyIndentPx", () => {
  const depths = [0, 1, 2, 3, 4, 5, 6, 7];

  it("leaves a phone room for the queue marker and four key characters at any depth", () => {
    for (const depth of depths) {
      expect(keyIndentPx(depth, true) + PHONE_KEY_PREFIX_PX + FIVE_CHARS_PX).toBeLessThanOrEqual(
        PHONE_KEY_COLUMN_PX,
      );
    }
    expect(depths.map((depth) => keyIndentPx(depth, true))).toEqual([0, 0, 6, 12, 12, 12, 12, 12]);
  });

  it("moves the key 20px per level on a wider screen, as before", () => {
    for (const depth of depths) {
      const shift = depth === 0 ? 0 : keyIndentPx(depth, false) + 6;
      expect(shift).toBe(20 * Math.min(depth, MAX_INDENT_DEPTH));
    }
  });
});
