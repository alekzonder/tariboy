import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { TeamImportProgress } from "./TeamImportProgress";

describe("TeamImportProgress", () => {
  it("renders an operation that has no steps yet", () => {
    render(
      <TeamImportProgress
        operation={{
          import_id: "imp-1",
          team: "dev",
          status: "pending",
          steps: null,
          updated_at: "2026-01-01T00:00:00Z",
        }}
        canRetry={false}
        onRetry={() => {}}
      />,
    );
    expect(screen.getByLabelText("Team import progress")).toHaveTextContent("Import status: pending");
  });
});
