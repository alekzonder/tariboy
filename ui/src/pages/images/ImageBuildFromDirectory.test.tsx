import { afterEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import ImageBuildFromDirectory from "./ImageBuildFromDirectory";

afterEach(() => vi.restoreAllMocks());

it("sends an explicit name and non-default tag while leaving sources in place", async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () => JSON.stringify({
      ok: true,
      result: { name: "reviewer", tag: "v3", digest: "sha256:new", layers: 2 },
    }),
  } as Response);
  vi.stubGlobal("fetch", fetchMock);
  render(<ImageBuildFromDirectory />);

  fireEvent.change(screen.getByLabelText("Image source directory"), { target: { value: "/srv/images/reviewer" } });
  fireEvent.change(screen.getByLabelText("Image name"), { target: { value: "reviewer" } });
  fireEvent.change(screen.getByLabelText("Image tag"), { target: { value: "v3" } });
  fireEvent.click(screen.getByRole("button", { name: "Build" }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
    "/api/images/build",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ path: "/srv/images/reviewer", name: "reviewer", tag: "v3" }),
    }),
  ));
});

it("shows the exact resolved template and warnings before build", async () => {
	const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () => JSON.stringify({
      ok: true,
      result: {
        valid: true,
        schema_version: 2,
        plugins: ["context"],
		skills: [{
			name: "code-review", description: "Review changes safely.",
			source: "./skills/code-review", category: "source",
			archive_root: "skills/code-review", file_count: 3, size: 2048,
			tree_sha256: "skill-tree-sha",
		}],
        diagnostics: [],
        warnings: [{ path: "prompts", message: "identity placeholder is omitted" }],
        template: {
          schema_version: 2,
          sha256: "template-sha",
          entries: [
            { kind: "runtime", runtime: "context" },
            { kind: "file", source: "./role.md", category: "source", archive_path: "prompt/layers/001-role.md", size: 42, sha256: "abcdef0123456789" },
          ],
        },
      },
    }),
	} as Response);
	vi.stubGlobal("fetch", fetchMock);
	render(<ImageBuildFromDirectory />);
	fireEvent.change(screen.getByLabelText("Image source directory"), { target: { value: "/srv/images/reviewer" } });
	fireEvent.change(screen.getByLabelText("Image name"), { target: { value: "reviewer" } });
	fireEvent.change(screen.getByLabelText("Image tag"), { target: { value: "v3" } });
	fireEvent.click(screen.getByRole("button", { name: "Validate" }));

  const preview = await screen.findByLabelText("Validated image template");
  const rows = within(preview).getAllByRole("listitem");
  expect(within(rows[0]).getByText("context")).toBeInTheDocument();
  expect(screen.getByText("./role.md")).toBeInTheDocument();
  expect(screen.getByText(/source · 42 bytes · abcdef0123456789/)).toBeInTheDocument();
  expect(screen.getByText(/identity placeholder is omitted/)).toBeInTheDocument();
	expect(screen.getByText("code-review")).toBeInTheDocument();
	expect(screen.getByText("Review changes safely.")).toBeInTheDocument();
	expect(screen.getByText(/source · \.\/skills\/code-review/)).toBeInTheDocument();
	expect(screen.getByText(/3 files · 2048 bytes · skill-tree-sha/)).toBeInTheDocument();
	expect(rows).toHaveLength(2);
	expect(fetchMock).toHaveBeenCalledWith("/api/images/validate", expect.objectContaining({
		method: "POST",
		body: JSON.stringify({ path: "/srv/images/reviewer", name: "reviewer", tag: "v3" }),
	}));
});

it("dismisses validation diagnostics and the resolved template", async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () => JSON.stringify({
      ok: true,
      result: {
        valid: true,
        schema_version: 2,
        diagnostics: [{ path: "skills", message: "duplicate skill name" }],
        warnings: [{ path: "prompts", message: "identity placeholder is omitted" }],
        template: { schema_version: 2, sha256: "template-sha", entries: [] },
      },
    }),
  } as Response);
  vi.stubGlobal("fetch", fetchMock);
  render(<ImageBuildFromDirectory />);

  fireEvent.change(screen.getByLabelText("Image source directory"), { target: { value: "/srv/images/reviewer" } });
  fireEvent.change(screen.getByLabelText("Image name"), { target: { value: "reviewer" } });
  fireEvent.click(screen.getByRole("button", { name: "Validate" }));

  expect(await screen.findByText(/duplicate skill name/)).toBeInTheDocument();
  expect(screen.getByLabelText("Validated image template")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));

  expect(screen.queryByText(/duplicate skill name/)).not.toBeInTheDocument();
  expect(screen.queryByText(/identity placeholder is omitted/)).not.toBeInTheDocument();
  expect(screen.queryByLabelText("Validated image template")).not.toBeInTheDocument();
});
