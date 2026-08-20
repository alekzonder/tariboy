import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ImageSkills from "./ImageSkills";

const useImageContext = vi.fn();
vi.mock("@/components/ImageLayout", () => ({ useImageContext: () => useImageContext() }));

describe("ImageSkills", () => {
  beforeEach(() => useImageContext.mockReset());

  it("shows packaged skills in manifest order with complete metadata", () => {
    useImageContext.mockReturnValue({
      manifest: { skills: [
        { name: "review", description: "Review changes.", source: "./skills/review", category: "source", archive_root: "skills/review", file_count: 3, size: 2048, tree_sha256: "aaa" },
        { name: "release", description: "Prepare releases.", source: "$PLUGINS/release", category: "plugin", archive_root: "skills/release", file_count: 2, size: 1024, tree_sha256: "bbb" },
      ] },
    });
    render(<ImageSkills />);
    const cards = screen.getAllByRole("listitem");
    expect(within(cards[0]).getByText("review")).toBeInTheDocument();
    expect(within(cards[0]).getByText("Review changes.")).toBeInTheDocument();
    expect(within(cards[0]).getByText("./skills/review")).toBeInTheDocument();
    expect(within(cards[0]).getByText(/source · skills\/review/)).toBeInTheDocument();
    expect(within(cards[0]).getByText(/3 files · 2048 bytes/)).toBeInTheDocument();
    expect(within(cards[0]).getByText("aaa")).toBeInTheDocument();
    expect(within(cards[1]).getByText("release")).toBeInTheDocument();
  });

  it("shows an empty state", () => {
    useImageContext.mockReturnValue({ manifest: { skills: [] } });
    render(<ImageSkills />);
    expect(screen.getByText("This image contains no packaged skills.")).toBeInTheDocument();
  });
});
