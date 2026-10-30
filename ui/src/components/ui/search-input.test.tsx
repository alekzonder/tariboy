import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SearchInput } from "./search-input";

describe("SearchInput", () => {
  it("disables text assistance on its search field", () => {
    render(<SearchInput aria-label="Search agents" />);

    const field = screen.getByRole("searchbox", { name: "Search agents" });
    expect(field).toHaveAttribute("spellcheck", "false");
    expect(field).toHaveAttribute("autocorrect", "off");
    expect(field).toHaveAttribute("autocapitalize", "off");
  });

  it("passes the value and change handler to the field", () => {
    const onChange = vi.fn();
    render(<SearchInput aria-label="Search tasks" value="dev" onChange={onChange} />);

    const field = screen.getByRole("searchbox", { name: "Search tasks" });
    expect(field).toHaveValue("dev");
    fireEvent.change(field, { target: { value: "devops" } });
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("shows an optional hint after the field", () => {
    render(<SearchInput aria-label="Search agents" hint="⌘K" />);

    expect(screen.getByText("⌘K")).toHaveAttribute("aria-hidden", "true");
  });
});
