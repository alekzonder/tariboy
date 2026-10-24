import { describe, expect, it } from "vitest";
import { transferStatusLabel, transferSummaryText, type TransferRow } from "./transferStatus";

describe("transferStatusLabel", () => {
  it.each<[TransferRow["status"], string]>([
    ["queued", "Queued"],
    ["exporting", "Exporting"],
    ["previewing", "Previewing"],
    ["importing", "Importing"],
    ["completed", "Completed"],
    ["already-present", "Already present"],
    ["failed", "Failed"],
    ["cancelled", "Cancelled"],
  ])("labels %s as %s", (status, label) => {
    expect(transferStatusLabel(status)).toBe(label);
  });
});

describe("transferSummaryText", () => {
  const idle = { phase: "idle" as const, rows: {}, targetCount: 2, selectedCount: 0 };

  it("reports the export while it runs", () => {
    expect(transferSummaryText({ ...idle, phase: "exporting" })).toBe("Exporting source archive.");
  });

  it("counts completed and already present targets while transferring", () => {
    const rows: Record<string, TransferRow> = {
      a: { status: "completed" },
      b: { status: "already-present" },
      c: { status: "failed" },
      d: { status: "queued" },
    };
    expect(transferSummaryText({ ...idle, phase: "transferring", rows })).toBe(
      "Transferring image: 2 targets completed.",
    );
    expect(
      transferSummaryText({ ...idle, phase: "transferring", rows: { a: { status: "completed" } } }),
    ).toBe("Transferring image: 1 target completed.");
  });

  it("says no targets are selected when none are eligible", () => {
    expect(transferSummaryText({ ...idle, targetCount: 0 })).toBe("No transfer targets selected.");
  });

  it("counts the selected targets when idle", () => {
    expect(transferSummaryText(idle)).toBe("0 transfer targets selected.");
    expect(transferSummaryText({ ...idle, selectedCount: 1 })).toBe("1 transfer target selected.");
  });
});
