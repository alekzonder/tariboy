import { expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import UsageSummaryCard from "./UsageSummaryCard";

it("renders the label as a term and the value as its description", () => {
  render(
    <dl>
      <UsageSummaryCard label="Requests" value="3" />
    </dl>,
  );
  expect(screen.getByRole("term")).toHaveTextContent("Requests");
  expect(screen.getByRole("definition")).toHaveTextContent("3");
});
