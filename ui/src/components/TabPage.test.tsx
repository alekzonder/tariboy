import { expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import TabPage from "./TabPage";

it("pads a full-width tab like Configuration", () => {
  render(<TabPage><p>content</p></TabPage>);
  const page = screen.getByTestId("tab-page");
  expect(page).toContainElement(screen.getByText("content"));
  expect(page).toHaveClass("px-4", "pt-4", "pb-6", "gap-4", "h-full");
  expect(page).not.toHaveClass("max-w-5xl");
});

it("centers a narrow tab in Configuration's column", () => {
  render(<TabPage narrow><p>content</p></TabPage>);
  expect(screen.getByTestId("tab-page")).toHaveClass("px-4", "pt-4", "pb-6", "mx-auto", "max-w-5xl");
});
