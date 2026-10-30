import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import { ApiProvider } from "@/components/ApiProvider";
import { addDaemon, type Daemon } from "@/lib/daemons";
import { FakeWebSocket } from "@/test/fakeWebSocket";
import { operationStarted } from "@/test/fixtures/apiFixtures";
import LibraryImagesPage from "./LibraryImagesPage";

interface Call {
  url: string;
  method: string;
  body?: string;
}

const base = { bare: false, exportable: true, schema_version: 1, pending_agents: null };
const images = [
  {
    ...base,
    name: "bare",
    tag: "latest",
    digest: "sha256:bare",
    built_at: "2026-07-29T00:00:00Z",
    bare: true,
    current_agents: ["shell"],
  },
  {
    ...base,
    name: "imported",
    tag: "v1",
    digest: "sha256:imported",
    built_at: "2026-07-29T00:00:00Z",
    current_agents: null,
  },
  {
    ...base,
    name: "reviewer",
    tag: "v2",
    image_version: "2.0.0",
    source: "tariboy",
    digest: "sha256:reviewer2",
    built_at: "2026-07-30T00:00:00Z",
    current_agents: ["r1"],
  },
  {
    ...base,
    name: "reviewer",
    tag: "latest",
    image_version: "1.0.0",
    digest: "sha256:reviewer",
    built_at: "2026-07-29T00:00:00Z",
    current_agents: ["r2"],
    pending_agents: ["r1"],
  },
  {
    ...base,
    name: "developer",
    tag: "latest",
    digest: "sha256:developer",
    built_at: "2026-07-29T00:00:00Z",
    current_agents: ["d1"],
  },
];

function stubApi(calls: Call[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init: RequestInit = {}) => {
      const method = init.method ?? "GET";
      calls.push({ url, method, body: typeof init.body === "string" ? init.body : undefined });
      let result: unknown = {};
      if (url.endsWith("/api/images") && method === "GET") {
        result = { images, count: images.length };
      } else if (url.endsWith("/api/images/validate") && method === "POST") {
        result = { valid: true, schema_version: 2, diagnostics: [] };
      } else if (url.endsWith("/api/images/build") && method === "POST") {
        result = operationStarted({ id: 3 });
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
}

function Location() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname + location.search}</div>;
}

// An explicit host is mounted the way RouteHostBoundary mounts a route host:
// as the ApiProvider of the page.
function renderPage(host?: Daemon) {
  const page = <LibraryImagesPage hostId={host?.id ?? ""} />;
  return render(
    <MemoryRouter initialEntries={["/library/images"]}>
      <DaemonProvider>
        <Routes>
          <Route
            path="/library/images"
            element={host ? <ApiProvider target={host}>{page}</ApiProvider> : page}
          />
          <Route path="*" element={null} />
        </Routes>
        <Location />
      </DaemonProvider>
    </MemoryRouter>,
  );
}

const rowNames = () =>
  screen
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).queryAllByRole("link")[0]?.textContent);

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  FakeWebSocket.install();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("Library images", () => {
  it("lists one row per used image, the most used first, without bare", async () => {
    stubApi([]);
    renderPage();
    expect(await screen.findByRole("link", { name: "reviewer" })).toHaveAttribute(
      "href",
      "/library/images/reviewer",
    );
    expect(rowNames()).toEqual(["reviewer", "developer"]);
    const headers = screen.getAllByRole("columnheader").map((cell) => cell.textContent);
    expect(headers).toEqual(["Image", "Version", "Source", "Used by", "Built", ""]);
    const row = screen.getByRole("link", { name: "reviewer" }).closest("tr")!;
    expect(row).toHaveTextContent("2.0.0");
    expect(row).toHaveTextContent("tariboy");
    expect(row).toHaveTextContent("2 agents");
    expect(within(row).getByRole("link", { name: "Run agent reviewer" })).toHaveAttribute(
      "href",
      "/?new=1&host=&image=reviewer%3Alatest",
    );
    expect(screen.queryByRole("link", { name: "bare" })).toBeNull();
  });

  it("shows unused images on request and filters by name", async () => {
    stubApi([]);
    renderPage();
    await screen.findByRole("link", { name: "reviewer" });
    const unused = screen.getByRole("checkbox", { name: "Show unused (1)" });
    expect(unused).not.toBeChecked();
    fireEvent.click(unused);
    expect(rowNames()).toEqual(["reviewer", "developer", "imported"]);
    fireEvent.change(screen.getByRole("searchbox", { name: "Search images" }), {
      target: { value: "DEV" },
    });
    expect(rowNames()).toEqual(["developer"]);
  });

  it("offers Show unused when the filter hides every image", async () => {
    stubApi([]);
    renderPage();
    await screen.findByRole("link", { name: "reviewer" });
    fireEvent.change(screen.getByRole("searchbox", { name: "Search images" }), {
      target: { value: "imp" },
    });
    expect(screen.getByText("No agent uses these images yet.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show unused" }));
    expect(rowNames()).toEqual(["imported"]);
  });

  it("builds from a directory in a dialog of the Add image menu", async () => {
    const calls: Call[] = [];
    stubApi(calls);
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("link", { name: "reviewer" });
    expect(screen.queryByLabelText("Image source directory")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Add image" }));
    await user.click(await screen.findByRole("menuitem", { name: "Build from directory" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Image source directory"), {
      target: { value: "/srv/images/reviewer" },
    });
    fireEvent.change(within(dialog).getByLabelText("Image name"), {
      target: { value: "reviewer" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "Build" }));
    await waitFor(() =>
      expect(calls).toContainEqual({
        url: "/api/images/build",
        method: "POST",
        body: JSON.stringify({ path: "/srv/images/reviewer", name: "reviewer", initiator: "ui" }),
      }),
    );
  });

  it("imports an archive in a dialog and opens Sources from the menu", async () => {
    stubApi([]);
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("link", { name: "reviewer" });
    await user.click(screen.getByRole("button", { name: "Add image" }));
    await user.click(await screen.findByRole("menuitem", { name: "Import archive" }));
    expect(
      within(await screen.findByRole("dialog")).getByLabelText("Import image archive"),
    ).toBeInTheDocument();
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "Add image" }));
    await user.click(await screen.findByRole("menuitem", { name: "Get from a source" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/library/sources");
  });

  it("loads built images from the explicit host", async () => {
    const host = {
      ...(await addDaemon({ label: "prod", baseURL: "https://prod:8765", token: "tp" })),
      token: "tp",
    };
    const calls: Call[] = [];
    stubApi(calls);
    renderPage(host);
    await screen.findByRole("link", { name: "reviewer" });
    expect(calls).toContainEqual(
      expect.objectContaining({ url: "https://prod:8765/api/images", method: "GET" }),
    );
  });
});
