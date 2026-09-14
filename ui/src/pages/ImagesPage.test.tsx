import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import { addDaemon } from "@/lib/daemons";
import ImagesPage from "./ImagesPage";

interface Call { url: string; method: string; body?: string }

const images = [
  {
    name: "bare", tag: "latest", digest: "sha256:bare", built_at: "2026-07-29T00:00:00Z",
    bare: true, exportable: true,
  },
  {
    name: "imported", tag: "v1", digest: "sha256:imported", built_at: "2026-07-29T00:00:00Z",
    bare: false, exportable: true,
  },
  {
    name: "reviewer", tag: "latest", digest: "sha256:reviewer", built_at: "2026-07-29T00:00:00Z",
    bare: false, exportable: true, source_cwd: "/srv/images/reviewer",
  },
];

function stubApi(calls: Call[]) {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string, init: RequestInit = {}) => {
    const method = init.method ?? "GET";
    calls.push({ url, method, body: typeof init.body === "string" ? init.body : undefined });
    let result: unknown = {};
    if (url.endsWith("/api/images") && method === "GET") {
      result = { images, count: images.length };
    } else if (url.endsWith("/api/images/validate") && method === "POST") {
      result = { valid: true, schema_version: 2, diagnostics: [] };
    } else if (url.endsWith("/api/images/build") && method === "POST") {
      result = { name: "reviewer", tag: "latest", digest: "sha256:new", layers: 3 };
    }
    return Promise.resolve({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ ok: true, result }),
    } as Response);
  }));
}

function renderPage(hostId = "") {
  return render(
    <MemoryRouter>
      <DaemonProvider>
        <ImagesPage hostId={hostId} />
      </DaemonProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => vi.restoreAllMocks());

describe("Images workspace", () => {
  it("builds from an original directory with required name and latest by default", async () => {
    const calls: Call[] = [];
    stubApi(calls);
    renderPage();

    expect(await screen.findByText("Build from directory")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Image source directory"), { target: { value: "/srv/images/reviewer" } });
    fireEvent.change(screen.getByLabelText("Image name"), { target: { value: "reviewer" } });
    expect(screen.getByLabelText("Image tag")).toHaveValue("latest");

    fireEvent.click(screen.getByRole("button", { name: "Validate" }));
    await waitFor(() => expect(calls).toContainEqual(expect.objectContaining({
      url: "/api/images/validate", method: "POST",
    })));

    fireEvent.click(screen.getByRole("button", { name: "Build" }));
    await waitFor(() => expect(calls).toContainEqual({
      url: "/api/images/build",
      method: "POST",
      body: JSON.stringify({ path: "/srv/images/reviewer", name: "reviewer" }),
    }));
  });

  it("lists image names with import at the root and tag actions behind the name", async () => {
    const calls: Call[] = [];
    stubApi(calls);
    renderPage();
    expect(await screen.findByRole("link", { name: "reviewer" })).toHaveAttribute("href", "/images/reviewer");
    expect(screen.getByRole("link", { name: "bare" })).toHaveAttribute("href", "/images/bare");
    expect(screen.getByLabelText("Import image archive")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Export/ })).toBeNull();
  });

  it("loads built images from the explicit host", async () => {
    const host = await addDaemon({ label: "prod", baseURL: "https://prod:8765", token: "tp" });
    const calls: Call[] = [];
    stubApi(calls);
    renderPage(host.id);
    await screen.findByRole("link", { name: "reviewer" });

    await waitFor(() => {
      expect(calls).toContainEqual(expect.objectContaining({
        url: "https://prod:8765/api/images",
        method: "GET",
      }));
    });
  });
});
