import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import { HostSwitcher } from "@/components/HostSwitcher";
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

function renderPage(withSwitcher = false) {
  return render(
    <MemoryRouter>
      <DaemonProvider>
        {withSwitcher && <HostSwitcher />}
        <ImagesPage />
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

  it("marks bare terminal-only and exposes runnable artifact actions", async () => {
    const calls: Call[] = [];
    stubApi(calls);
    renderPage();

    const bare = await screen.findByTestId("built-image-bare:latest");
    expect(within(bare).getByText("Terminal-only")).toBeInTheDocument();
    expect(within(bare).queryByRole("button", { name: /Remove/i })).not.toBeInTheDocument();
    expect(within(bare).queryByRole("button", { name: /Export/i })).not.toBeInTheDocument();

    const imported = screen.getByTestId("built-image-imported:v1");
    expect(within(imported).getByText("Source CWD unavailable — imported artifact")).toBeInTheDocument();
    expect(within(imported).getByRole("button", { name: "Export imported:v1" })).toBeEnabled();

    const built = screen.getByTestId("built-image-reviewer:latest");
    expect(within(built).getByText("/srv/images/reviewer")).toBeInTheDocument();
    expect(within(built).getByRole("link", { name: "Run Agent" })).toHaveAttribute(
      "href",
      "/?new=1&host=&image=reviewer%3Alatest",
    );
  });

  it("reloads built images through the host selected by HostSwitcher", async () => {
    await addDaemon({ label: "prod", baseURL: "https://prod:8765", token: "tp" });
    const calls: Call[] = [];
    stubApi(calls);
    renderPage(true);

    await screen.findByTestId("built-image-reviewer:latest");
    fireEvent.click(screen.getByRole("combobox"));
    fireEvent.click(await screen.findByRole("option", { name: "prod" }));

    await waitFor(() => {
      expect(calls).toContainEqual(expect.objectContaining({
        url: "https://prod:8765/api/images",
        method: "GET",
      }));
    });
  });
});
