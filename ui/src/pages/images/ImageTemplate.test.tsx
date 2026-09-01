import { afterEach, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import ImageTemplate from "./ImageTemplate";

vi.mock("@/components/ImageLayout", () => ({
  useImageContext: () => ({ ref: "reviewer:v2", hostKey: "", manifest: null, provenance: null }),
}));

afterEach(() => vi.restoreAllMocks());

it("renders static layers and runtime placeholders in declared order", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () => JSON.stringify({
      ok: true,
      result: {
        schema_version: 2,
        sha256: "template-sha",
        entries: [
          {
            kind: "file", source: "$STORE/prompts/iteration-finish.md", category: "store",
            archive_path: "prompt/layers/000-iteration-finish.md", size: 220, sha256: "abcdef0123456789",
          },
          { kind: "runtime", runtime: "identity" },
          {
            kind: "file", source: "/srv/prompts/reviewer.md", category: "absolute",
            archive_path: "prompt/layers/002-reviewer.md", size: 42, sha256: "9876543210fedcba",
          },
        ],
      },
    }),
  } as Response));

  render(<ImageTemplate />);

  expect(await screen.findByText(/template sha256 template-sha/)).toBeInTheDocument();
  const rows = screen.getAllByRole("listitem");
  expect(rows).toHaveLength(3);
  expect(within(rows[0]).getByText("$STORE/prompts/iteration-finish.md")).toBeInTheDocument();
  expect(within(rows[1]).getByText("identity")).toBeInTheDocument();
  expect(within(rows[2]).getByText("/srv/prompts/reviewer.md")).toBeInTheDocument();
  expect(vi.mocked(fetch)).toHaveBeenCalledWith(
    "/api/images/reviewer%3Av2/template",
    expect.objectContaining({ method: "GET" }),
  );
});
