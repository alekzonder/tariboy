import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { ImageLayout } from "./ImageLayout";

const { openHostPathInVSCode } = vi.hoisted(() => ({
  openHostPathInVSCode: vi.fn().mockResolvedValue(null),
}));
vi.mock("@/lib/desktop", () => ({ openHostPathInVSCode }));

// Serve the manifest for GET /api/images/{ref}; the tab bodies below are stubs
// so this test covers only the layout's header + tab routing, not the tabs'
// own fetches.
function stubFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string) => {
      const path = String(url);
      let result: unknown = {};
      if (path.endsWith("/api/images")) {
        result = {
          images: [
            {
              name: "foo", tag: "v1", digest: "sha256:deadbeefcafef00d",
              built_at: "2026-07-12T00:00:00Z", bare: false, source: "foo-source",
            },
            {
              name: "bare", tag: "latest", digest: "sha256:bare",
              built_at: "2026-07-12T00:00:00Z", bare: true,
            },
          ],
          count: 2,
        };
      } else if (/\/api\/images\/[^/]+$/.test(path)) {
        const bare = path.includes("bare%3Alatest");
        result = {
          schema_version: 1, name: bare ? "bare" : "foo", tag: bare ? "latest" : "v1",
          digest: bare ? "sha256:bare" : "sha256:deadbeefcafef00d",
          built_at: "2026-07-12T00:00:00Z", parents: [], plugins: [], requires_secrets: [],
          harness: { type: "claude", interactive: false }, env: {},
          policy: {}, evals: [], layers: [], bare,
        };
      } else if (path.endsWith("/provenance")) {
        result = path.includes("bare%3Alatest")
          ? { ref: "bare:latest", source_cwd: null, source_available: false }
          : { ref: "foo:v1", source_cwd: "/srv/images/foo", source_available: true };
      }
      return Promise.resolve({
        ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
}

beforeEach(() => stubFetch());
afterEach(() => vi.restoreAllMocks());

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/images/:name/:tag" element={<ImageLayout />}>
          <Route index element={<div>overview body</div>} />
          <Route path="template" element={<div>template body</div>} />
		  <Route path="skills" element={<div>skills body</div>} />
          <Route path="files" element={<div>files body</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

function renderServerAt(path: string) {
  function LocationProbe() {
    const location = useLocation();
    return <output data-testid="location">{location.pathname + location.search}</output>;
  }
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LocationProbe />
      <Routes>
        <Route
          path="/servers/remote-1/images/:name/:tag"
          element={<ImageLayout hostId="remote-1" basePath="/servers/remote-1/images" />}
        >
          <Route index element={<div>overview body</div>} />
          <Route path="template" element={<div>template body</div>} />
		  <Route path="skills" element={<div>skills body</div>} />
          <Route path="files" element={<div>files body</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe("ImageLayout", () => {
  it("renders the ref header and tab strip, routing to the index (Overview) tab", async () => {
    renderAt("/images/foo/v1");
    expect(screen.getByText("overview body")).toBeInTheDocument();
    expect(screen.getByText("foo:v1")).toBeInTheDocument();
    // All three tabs are present.
    expect(screen.getByText("Overview")).toBeInTheDocument();
    expect(screen.getByText("Template")).toBeInTheDocument();
	expect(screen.getByText("Skills")).toBeInTheDocument();
    expect(screen.getByText("Files")).toBeInTheDocument();
    // The header shows the short digest once the manifest loads.
    await waitFor(() => expect(screen.getByText("sha256:deadbeefc")).toBeInTheDocument());
  });

  it("routes to the Template tab", () => {
    renderAt("/images/foo/v1/template");
    expect(screen.getByText("template body")).toBeInTheDocument();
    expect(screen.queryByText("overview body")).not.toBeInTheDocument();
  });

  it("routes to the Files tab", () => {
    renderAt("/images/foo/v1/files");
    expect(screen.getByText("files body")).toBeInTheDocument();
    expect(screen.queryByText("overview body")).not.toBeInTheDocument();
  });

  it("routes to the Skills tab", () => {
	renderAt("/images/foo/v1/skills");
	expect(screen.getByText("skills body")).toBeInTheDocument();
	expect(screen.getByRole("link", { name: "Skills" })).toHaveAttribute("href", "/images/foo/v1/skills");
  });

  it("offers run, source CWD, and remove for an image built from a directory", async () => {
    renderAt("/images/foo/v1");
    expect(await screen.findByRole("link", { name: "Run Agent" }))
      .toHaveAttribute("href", "/?new=1&host=&image=foo%3Av1");
    expect(await screen.findByText("/srv/images/foo")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Open in VS Code" }));
    expect(openHostPathInVSCode).toHaveBeenCalledWith("", "/srv/images/foo");
    expect(screen.getByRole("button", { name: "Remove foo:v1" })).toBeInTheDocument();
  });

  it("makes bare terminal-only and read-only", async () => {
    renderAt("/images/bare/latest");
    expect(await screen.findByText("Terminal-only")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Rebuild/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Remove bare/i })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Run Agent" })).toBeInTheDocument();
  });

  it("keeps image actions scoped to an explicit server route", async () => {
    renderServerAt("/servers/remote-1/images/foo/v1");

    expect(await screen.findByRole("link", { name: "Run Agent" }))
      .toHaveAttribute("href", "/?new=1&host=remote-1&image=foo%3Av1");
    fireEvent.click(screen.getByRole("link", { name: "Template" }));
    expect(screen.getByTestId("location"))
      .toHaveTextContent("/servers/remote-1/images/foo/v1/template");
	fireEvent.click(screen.getByRole("link", { name: "Skills" }));
	expect(screen.getByTestId("location"))
	  .toHaveTextContent("/servers/remote-1/images/foo/v1/skills");
  });
});
