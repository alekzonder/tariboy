import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RoleSection } from "./RoleSection";

function contentHeight(height: number) {
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(height);
}

afterEach(() => vi.restoreAllMocks());

describe("RoleSection", () => {
  it("names the section by its Role heading and cites the file", () => {
    render(<RoleSection variant="card" text="You review code." empty="none" />);
    const section = screen.getByRole("region", { name: "Role" });
    expect(section.tagName).toBe("SECTION");
    expect(section).toHaveTextContent("From ROLE.md");
    expect(section).toHaveTextContent("You review code.");
  });

  it("shows the empty line when there is no ROLE.md", () => {
    render(
      <RoleSection variant="section" text={null} empty="This workflow image has no ROLE.md." />,
    );
    const empty = screen.getByText("This workflow image has no ROLE.md.");
    expect(empty.className).toContain("text-muted-foreground");
    expect(screen.queryByText("From ROLE.md")).not.toBeInTheDocument();
  });

  it("shows a text of 320px or less whole, with no button", () => {
    contentHeight(320);
    render(<RoleSection variant="card" text="Short." empty="none" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(document.getElementById(screen.getByText("Short.").closest("[id]")!.id)).not.toHaveClass(
      "max-h-60",
    );
  });

  it("folds a taller text to 240px behind Show all and unfolds it", () => {
    contentHeight(321);
    render(<RoleSection variant="card" text="Long." empty="none" />);
    const button = screen.getByRole("button", { name: "Show all" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    const body = document.getElementById(button.getAttribute("aria-controls")!)!;
    expect(body).toHaveClass("max-h-60");
    expect(body.parentElement!.querySelector("[aria-hidden=true]")).toHaveClass("h-12");

    fireEvent.click(button);
    expect(button).toHaveAccessibleName("Show less");
    expect(button).toHaveAttribute("aria-expanded", "true");
    expect(body).not.toHaveClass("max-h-60");
    expect(body.parentElement!.querySelector("[aria-hidden=true]")).toBeNull();
  });

  it("unfolds when focus moves into the folded text", () => {
    contentHeight(500);
    render(<RoleSection variant="card" text="[link](https://x.test)" empty="none" />);
    fireEvent.focus(screen.getByRole("link", { name: "link" }));
    expect(screen.getByRole("button", { name: "Show less" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("scrolls back to the heading on Show less only when it is above the screen", () => {
    contentHeight(500);
    const scroll = vi.spyOn(Element.prototype, "scrollIntoView").mockImplementation(() => {});
    render(<RoleSection variant="section" text="Long." empty="none" />);
    const heading = screen.getByRole("heading", { name: "Role" });
    fireEvent.click(screen.getByRole("button", { name: "Show all" }));
    fireEvent.click(screen.getByRole("button", { name: "Show less" }));
    expect(scroll).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Show all" }));
    vi.spyOn(heading, "getBoundingClientRect").mockReturnValue({ top: -40 } as DOMRect);
    const button = screen.getByRole("button", { name: "Show less" });
    button.focus();
    fireEvent.click(button);
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(scroll.mock.contexts[0]).toBe(heading);
    expect(button).toHaveFocus();
  });
});
