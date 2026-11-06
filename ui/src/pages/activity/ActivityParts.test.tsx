import { describe, expect, it } from "vitest";
import { render } from "@/test/queryRender";
import { ActivityTotals } from "./ActivityParts";

// jsdom does not lay out: this pins the classes that keep the numbers of the
// tiles on one line when a label wraps (Status changes at 1024 px).
describe("ActivityTotals tile", () => {
  it("pushes the number to the bottom of a column tile", () => {
    const { container } = render(<ActivityTotals totals={null} />);
    const tile = container.querySelector('[data-total="Status changes"]');
    expect(tile).toHaveClass("flex", "flex-col");
    expect(tile?.lastElementChild).toHaveClass("mt-auto");
  });
});
