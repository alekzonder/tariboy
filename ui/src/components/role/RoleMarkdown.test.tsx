import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import RoleMarkdown from "./RoleMarkdown";

const { openExternalUrl } = vi.hoisted(() => ({
  openExternalUrl: vi.fn().mockResolvedValue(null),
}));

vi.mock("@/lib/desktop", () => ({
  isDesktop: () => true,
  opensWebLinks: () => true,
  openExternalUrl,
}));

describe("RoleMarkdown", () => {
  it("draws smaller headings than a task description", () => {
    const { container } = render(<RoleMarkdown text={"# Reviewer\n\n## Duties\n\n### Notes"} />);
    const classes = (container.firstElementChild as HTMLElement).className.split(" ");
    expect(classes).toEqual(
      expect.arrayContaining([
        "[&_h1]:text-heading",
        "[&_h2]:text-title",
        "[&_:is(h3,h4,h5,h6)]:text-body",
      ]),
    );
    expect(classes).not.toContain("[&_h1]:text-display");
    expect(screen.getByRole("heading", { level: 1, name: "Reviewer" })).toBeInTheDocument();
  });

  it("shows the alt text of an image instead of loading it", () => {
    const { container } = render(<RoleMarkdown text="![Team logo](https://x.test/logo.png)" />);
    expect(container.querySelector("img")).toBeNull();
    const alt = screen.getByText("Team logo");
    expect(alt.className).toContain("italic");
    expect(alt.className).toContain("text-muted-foreground");
  });

  it("never renders raw HTML", () => {
    const { container } = render(
      <RoleMarkdown
        text={'<b>bold</b>\n\n<img src="x" onerror="alert(1)">\n\n<script>alert(1)</script>'}
      />,
    );
    expect(container.querySelector("b, img, script")).toBeNull();
  });

  it("opens web links as task Markdown does", () => {
    openExternalUrl.mockClear();
    render(<RoleMarkdown text="[guide](https://example.com/guide)" />);
    const click = new MouseEvent("click", { bubbles: true, cancelable: true });
    screen.getByRole("link", { name: "guide" }).dispatchEvent(click);
    expect(click.defaultPrevented).toBe(true);
    expect(openExternalUrl).toHaveBeenCalledWith("https://example.com/guide");
  });
});
