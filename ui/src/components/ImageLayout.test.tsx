import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import * as daemons from "@/lib/daemons";
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
      const path = String(url).split("?")[0];
      let result: unknown = {};
      if (path.endsWith("/api/images")) {
        result = {
          images: [
            {
              name: "foo",
              tag: "v1",
              digest: "sha256:deadbeefcafef00d",
              built_at: "2026-07-12T00:00:00Z",
              bare: false,
              source: "foo-source",
            },
            {
              name: "bare",
              tag: "latest",
              digest: "sha256:bare",
              built_at: "2026-07-12T00:00:00Z",
              bare: true,
            },
          ],
          count: 2,
        };
      } else if (!path.endsWith("/provenance") && /\/api\/images\/[^/]+$/.test(path)) {
        const bare = path.includes("bare%3Alatest");
        result = {
          schema_version: 1,
          name: bare ? "bare" : "foo",
          tag: bare ? "latest" : "v1",
          digest: bare ? "sha256:bare" : "sha256:deadbeefcafef00d",
          built_at: "2026-07-12T00:00:00Z",
          parents: [],
          plugins: [],
          requires_secrets: [],
          harness: { type: "claude", interactive: false },
          env: {},
          policy: {},
          layers: [],
          bare,
        };
      } else if (path.endsWith("/provenance")) {
        result = path.includes("bare%3Alatest")
          ? { ref: "bare:latest", source_cwd: null, source_available: false }
          : { ref: "foo:v1", source_cwd: "/srv/images/foo", source_available: true };
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
}

beforeEach(() => {
  stubFetch();
  vi.spyOn(daemons, "resolveDaemon").mockImplementation(async (id) =>
    id ? { id, label: id, baseURL: "https://route", token: "" } : null,
  );
});
afterEach(() => vi.restoreAllMocks());

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/library/images/:name/:tag" element={<ImageLayout />}>
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
        <Route path="/library/images/:name/:tag" element={<ImageLayout hostId="remote-1" />}>
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
    renderAt("/library/images/foo/v1");
    expect(await screen.findByText("overview body")).toBeInTheDocument();
    expect(screen.getByText("foo:v1")).toBeInTheDocument();
    // All three tabs are present.
    expect(screen.getByText("Overview")).toBeInTheDocument();
    expect(screen.getByText("Template")).toBeInTheDocument();
    expect(screen.getByText("Skills")).toBeInTheDocument();
    expect(screen.getByText("Files")).toBeInTheDocument();
    // The header shows the short digest once the manifest loads.
    await waitFor(() => expect(screen.getByText("sha256:deadbeefc")).toBeInTheDocument());
  });

  it("routes to the Template tab", async () => {
    renderAt("/library/images/foo/v1/template");
    expect(await screen.findByText("template body")).toBeInTheDocument();
    expect(screen.queryByText("overview body")).not.toBeInTheDocument();
  });

  it("routes to the Files tab", async () => {
    renderAt("/library/images/foo/v1/files");
    expect(await screen.findByText("files body")).toBeInTheDocument();
    expect(screen.queryByText("overview body")).not.toBeInTheDocument();
  });

  it("routes to the Skills tab", async () => {
    renderAt("/library/images/foo/v1/skills");
    expect(await screen.findByText("skills body")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Skills" })).toHaveAttribute(
      "href",
      "/library/images/foo/v1/skills",
    );
  });

  it("offers run, source CWD, and remove for an image built from a directory", async () => {
    renderAt("/library/images/foo/v1");
    expect(await screen.findByRole("link", { name: "Run Agent" })).toHaveAttribute(
      "href",
      "/?new=1&host=&image=foo%3Av1",
    );
    expect(await screen.findByText("/srv/images/foo")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Open in VS Code" }));
    expect(openHostPathInVSCode).toHaveBeenCalledWith("", "/srv/images/foo");
    // The header and the tag's row in Versions both offer it.
    expect(await screen.findAllByRole("button", { name: "Remove foo:v1" })).toHaveLength(2);
  });

  it("makes bare terminal-only and read-only", async () => {
    renderAt("/library/images/bare/latest");
    expect(await screen.findByText("Terminal-only")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Rebuild/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Remove bare/i })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Run Agent" })).toBeInTheDocument();
  });

  it("keeps image actions scoped to an explicit server route", async () => {
    renderServerAt("/library/images/foo/v1");

    expect(await screen.findByRole("link", { name: "Run Agent" })).toHaveAttribute(
      "href",
      "/?new=1&host=remote-1&image=foo%3Av1",
    );
    fireEvent.click(screen.getByRole("link", { name: "Template" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/library/images/foo/v1/template");
    fireEvent.click(screen.getByRole("link", { name: "Skills" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/library/images/foo/v1/skills");
  });

  it("shows the Versions below the Overview only", async () => {
    renderAt("/library/images/foo/v1");
    expect(await screen.findByTestId("built-image-foo:v1")).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("region", { name: "Versions" })).toBeInTheDocument();
    renderAt("/library/images/foo/v1/template");
    expect(await screen.findByText("template body")).toBeInTheDocument();
    expect(screen.getAllByRole("region", { name: "Versions" })).toHaveLength(1);
  });

  it("opens the newest tag of a name with links to that tag's tabs", async () => {
    render(
      <MemoryRouter initialEntries={["/library/images/foo"]}>
        <Routes>
          <Route path="/library/images/:name" element={<ImageLayout />}>
            <Route index element={<div>overview body</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );
    expect(await screen.findByText("overview body")).toBeInTheDocument();
    expect(screen.getByText("foo:v1")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Template" })).toHaveAttribute(
      "href",
      "/library/images/foo/v1/template",
    );
    expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute("aria-current", "page");
    expect(await screen.findByRole("region", { name: "Versions" })).toBeInTheDocument();
  });

  it("says when a name has no tags left", async () => {
    render(
      <MemoryRouter initialEntries={["/library/images/gone"]}>
        <Routes>
          <Route path="/library/images/:name" element={<ImageLayout />} />
        </Routes>
      </MemoryRouter>,
    );
    expect(await screen.findByText("No tags for this image on this host.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Images" })).toHaveAttribute("href", "/library/images");
  });
});
