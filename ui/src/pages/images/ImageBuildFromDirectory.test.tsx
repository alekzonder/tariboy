import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ComponentType, PropsWithChildren } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { createApi, setActiveDaemon } from "@/lib/api";
import { apiWrapper } from "@/test/api";
import { FakeWebSocket } from "@/test/fakeWebSocket";
import { operationStarted } from "@/test/fixtures/apiFixtures";
import ImageBuildFromDirectory from "./ImageBuildFromDirectory";

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast }));

beforeEach(() => {
  vi.clearAllMocks();
  FakeWebSocket.install();
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  setActiveDaemon(null);
});

function OperationRoute() {
  return <p>operation {useLocation().pathname}</p>;
}

/** Mount under /servers/local/images, beside the operation route the toast links to. */
function withRouter(Api: ComponentType<PropsWithChildren>) {
  return function RouterWrapper({ children }: PropsWithChildren) {
    return (
      <Api>
        <MemoryRouter initialEntries={["/servers/local/images"]}>
          <Routes>
            <Route path="/servers/:hostId/images" element={children} />
            <Route path="/settings/maintenance/operations/:id" element={<OperationRoute />} />
          </Routes>
        </MemoryRouter>
      </Api>
    );
  };
}

it("sends an explicit name and non-default tag while leaving sources in place", async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () =>
      JSON.stringify({
        ok: true,
        result: operationStarted({ id: 4, title: "Build image reviewer:v3" }),
      }),
  } as Response);
  vi.stubGlobal("fetch", fetchMock);
  render(<ImageBuildFromDirectory />, { wrapper: withRouter(apiWrapper()) });

  fireEvent.change(screen.getByLabelText("Image source directory"), {
    target: { value: "/srv/images/reviewer" },
  });
  fireEvent.change(screen.getByLabelText("Image name"), { target: { value: "reviewer" } });
  fireEvent.change(screen.getByLabelText("Image tag"), { target: { value: "v3" } });
  fireEvent.click(screen.getByRole("button", { name: "Build" }));

  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/images/build",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          path: "/srv/images/reviewer",
          name: "reviewer",
          tag: "v3",
          initiator: "ui",
        }),
      }),
    ),
  );
  await waitFor(() => expect(toast.success).toHaveBeenCalled());
  const [message, options] = toast.success.mock.calls[0];
  expect(message).toBe("Build started");
  expect(options.description).toBe("Build image reviewer:v3");
  expect(options.action.label).toBe("Open operation");
  act(() => options.action.onClick());
  expect(await screen.findByText("operation /settings/maintenance/operations/4")).toBeTruthy();
});

it("reports a refused build as an error", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      text: async () =>
        JSON.stringify({ ok: false, error: { code: "bad_request", message: "no Tariboyfile" } }),
    } as Response),
  );
  render(<ImageBuildFromDirectory />, { wrapper: withRouter(apiWrapper()) });
  fireEvent.change(screen.getByLabelText("Image source directory"), {
    target: { value: "/srv/images/reviewer" },
  });
  fireEvent.change(screen.getByLabelText("Image name"), { target: { value: "reviewer" } });
  fireEvent.click(screen.getByRole("button", { name: "Build" }));
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith("build failed: no Tariboyfile"));
  expect(toast.success).not.toHaveBeenCalled();
});

it("shows the exact resolved template and warnings before build", async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () =>
      JSON.stringify({
        ok: true,
        result: {
          valid: true,
          schema_version: 2,
          plugins: ["context"],
          skills: [
            {
              name: "code-review",
              description: "Review changes safely.",
              source: "./skills/code-review",
              category: "source",
              archive_root: "skills/code-review",
              file_count: 3,
              size: 2048,
              tree_sha256: "skill-tree-sha",
            },
          ],
          diagnostics: [],
          warnings: [{ path: "prompts", message: "identity placeholder is omitted" }],
          template: {
            schema_version: 2,
            sha256: "template-sha",
            entries: [
              { kind: "runtime", runtime: "context" },
              {
                kind: "file",
                source: "./role.md",
                category: "source",
                archive_path: "prompt/layers/001-role.md",
                size: 42,
                sha256: "abcdef0123456789",
              },
            ],
          },
        },
      }),
  } as Response);
  vi.stubGlobal("fetch", fetchMock);
  render(<ImageBuildFromDirectory />, { wrapper: withRouter(apiWrapper()) });
  fireEvent.change(screen.getByLabelText("Image source directory"), {
    target: { value: "/srv/images/reviewer" },
  });
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
  expect(fetchMock).toHaveBeenCalledWith(
    "/api/images/validate",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ path: "/srv/images/reviewer", name: "reviewer", tag: "v3" }),
    }),
  );
});

it("dismisses validation diagnostics and the resolved template", async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () =>
      JSON.stringify({
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
  render(<ImageBuildFromDirectory />, { wrapper: withRouter(apiWrapper()) });

  fireEvent.change(screen.getByLabelText("Image source directory"), {
    target: { value: "/srv/images/reviewer" },
  });
  fireEvent.change(screen.getByLabelText("Image name"), { target: { value: "reviewer" } });
  fireEvent.click(screen.getByRole("button", { name: "Validate" }));

  expect(await screen.findByText(/duplicate skill name/)).toBeInTheDocument();
  expect(screen.getByLabelText("Validated image template")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));

  expect(screen.queryByText(/duplicate skill name/)).not.toBeInTheDocument();
  expect(screen.queryByText(/identity placeholder is omitted/)).not.toBeInTheDocument();
  expect(screen.queryByLabelText("Validated image template")).not.toBeInTheDocument();
});

it("keeps validation and build on the explicit target when the active host differs", async () => {
  const target = { id: "route", label: "Route", baseURL: "https://route", token: "" };
  setActiveDaemon({ id: "other", label: "Other", baseURL: "https://other", token: "" });
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      calls.push(String(input));
      return new Response(
        JSON.stringify({
          ok: true,
          result: { valid: true, schema_version: 2, name: "reviewer", tag: "latest" },
        }),
      );
    }),
  );
  render(<ImageBuildFromDirectory />, { wrapper: withRouter(apiWrapper(createApi(target))) });
  fireEvent.change(screen.getByLabelText("Image source directory"), {
    target: { value: "/srv/image" },
  });
  fireEvent.change(screen.getByLabelText("Image name"), { target: { value: "reviewer" } });
  fireEvent.click(screen.getByRole("button", { name: "Validate" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Build" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "Build" }));
  await waitFor(() =>
    expect(calls).toEqual(["https://route/api/images/validate", "https://route/api/images/build"]),
  );
  setActiveDaemon(null);
});
