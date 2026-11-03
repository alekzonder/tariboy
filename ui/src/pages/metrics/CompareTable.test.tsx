import { describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import type { CompareRow } from "./compareModel";
import { CompareTable } from "./CompareTable";

const row = (id: string, better: CompareRow["better"], a: number, b: number): CompareRow => ({
  id,
  label: id,
  format: "avg",
  better,
  a,
  b,
});

/** The Change cell of one row in the table. */
function change(id: string) {
  const table = screen.getByRole("table", { name: "Version comparison" });
  const cells = within(table).getByRole("rowheader", { name: id }).closest("tr")!.cells;
  return cells[cells.length - 1];
}

describe("CompareTable Change", () => {
  it("points the arrow by the sign of B − A and colors arrow and word by the verdict", () => {
    render(
      <CompareTable
        a="1"
        b="2"
        rows={[
          row("higher-up", "higher", 70, 80),
          row("higher-down", "higher", 80, 70),
          row("lower-up", "lower", 2, 3),
          row("lower-down", "lower", 3, 2),
        ]}
      />,
    );
    const cases: Array<[string, string, string, string]> = [
      ["higher-up", "lucide-arrow-up", "better", "text-status-running"],
      ["higher-down", "lucide-arrow-down", "worse", "text-destructive"],
      ["lower-up", "lucide-arrow-up", "worse", "text-destructive"],
      ["lower-down", "lucide-arrow-down", "better", "text-status-running"],
    ];
    for (const [id, icon, verdict, color] of cases) {
      const cell = change(id);
      expect(cell.querySelector("svg")!.getAttribute("class"), id).toContain(icon);
      const word = within(cell).getByText(verdict);
      expect(word.className, id).toContain(color);
      expect(word.contains(cell.querySelector("svg")), id).toBe(true);
    }
  });
});
