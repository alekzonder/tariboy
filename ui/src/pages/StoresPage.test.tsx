import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import { CustomerQuestionNotificationsContext } from "@/components/customerQuestionNotificationsContext";
import { SidebarStateProvider } from "@/pages/terminals/SidebarStateProvider";
import TerminalsPage from "@/pages/terminals/TerminalsPage";
import { fetchAllAgents } from "@/lib/aggregate";
import { setActiveDaemon } from "@/lib/api";
import { addDaemon } from "@/lib/daemons";

vi.mock("@/lib/aggregate", () => ({ fetchAllAgents: vi.fn() }));

interface Call {
  url: string;
  method: string;
  body?: string;
  authorization?: string;
}

const envelope = (result: unknown, ok = true): Response => ({
  ok,
  status: ok ? 200 : 500,
  text: async () => JSON.stringify(ok
    ? { ok: true, result }
    : { ok: false, error: { code: "store_failed", message: String(result) } }),
}) as Response;

function RouteButtons({ hostParam }: { hostParam: string }) {
  const navigate = useNavigate();
  return <>
    <button onClick={() => navigate(`/servers/${hostParam}/stores/old`)}>Open old</button>
    <button onClick={() => navigate(`/servers/${hostParam}/stores/new`)}>Open new</button>
  </>;
}

function renderPage(path: string, hostParam: string) {
  return render(
    <DaemonProvider>
      <CustomerQuestionNotificationsContext.Provider value={{ attention: new Set(), refreshHost: async () => {} }}>
        <SidebarStateProvider>
          <MemoryRouter initialEntries={[path]}>
            <RouteButtons hostParam={hostParam} />
            <Routes>
              <Route path="/servers/:hostId/stores" element={<TerminalsPage serverView="stores" />} />
              <Route path="/servers/:hostId/stores/:name" element={<TerminalsPage serverView="store-detail" />} />
            </Routes>
          </MemoryRouter>
        </SidebarStateProvider>
      </CustomerQuestionNotificationsContext.Provider>
    </DaemonProvider>,
  );
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

afterEach(() => {
  setActiveDaemon(null);
  vi.restoreAllMocks();
});

describe("Stores workspace", () => {
  it("keeps list and lifecycle actions on the route-selected host", async () => {
    const host = await addDaemon({
      label: "Store host",
      baseURL: "https://store.example",
      token: "store-token",
    });
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
      { host: { id: host.id, label: host.label }, agents: [] },
    ]);
    const calls: Call[] = [];
    let removeAttempts = 0;
    let finishBuild!: (response: Response) => void;
    const buildResponse = new Promise<Response>((resolve) => { finishBuild = resolve; });
    const imageBuilt = vi.fn();
    window.addEventListener("tariboy:image-built", imageBuilt);
    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
      const url = String(input);
      const method = init.method ?? "GET";
      const headers = new Headers(init.headers);
      calls.push({
        url,
        method,
        body: typeof init.body === "string" ? init.body : undefined,
        authorization: headers.get("Authorization") ?? undefined,
      });
      if (url.endsWith("/api/stores") && method === "GET") {
        return Promise.resolve(envelope([{ name: "team", source: "/srv/team", path: "/srv/team" }]));
      }
      if (url.endsWith("/api/stores") && method === "POST") {
        return Promise.resolve(envelope({ name: "design", source: "git@example.com:design.git", path: "/stores/design" }));
      }
      if (url.endsWith("/api/stores/design") && method === "GET") {
        return Promise.resolve(envelope({
          name: "design",
          source: "git@example.com:design.git",
          path: "/stores/design",
          images: [{ name: "reviewer", version: "1.2.3" }, { name: "broken", version: "latest", error: "invalid Tariboyfile" }],
        }));
      }
      if (url.endsWith("/api/stores/design/refresh")) {
        return Promise.resolve(envelope("refresh failed", false));
      }
      if (url.endsWith("/api/images/build")) return buildResponse;
      if (url.endsWith("/api/stores/design") && method === "DELETE") {
        removeAttempts++;
        return Promise.resolve(removeAttempts === 1
          ? envelope("remove failed", false)
          : envelope({ removed: true }));
      }
      return Promise.resolve(envelope({ agents: [], groups: [], count: 0 }));
    }));

    renderPage(`/servers/${encodeURIComponent(host.id)}/stores`, encodeURIComponent(host.id));

    expect(await screen.findByRole("heading", { name: "Stores" })).toBeInTheDocument();
    expect(await screen.findByText("/srv/team")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Images" })).toHaveAttribute(
      "href", `/servers/${encodeURIComponent(host.id)}/images`,
    );
    expect(screen.getByRole("link", { name: "Stores" })).toHaveAttribute(
      "href", `/servers/${encodeURIComponent(host.id)}/stores`,
    );

    fireEvent.change(screen.getByLabelText("Store name"), { target: { value: "design" } });
    fireEvent.change(screen.getByLabelText("Store source"), { target: { value: "git@example.com:design.git" } });
    fireEvent.click(screen.getByRole("button", { name: "Add store" }));

    expect(await screen.findByRole("heading", { name: "design" })).toBeInTheDocument();
    expect(await screen.findByText("1.2.3")).toBeInTheDocument();
    expect(screen.getByText("invalid Tariboyfile")).toBeInTheDocument();
    expect(calls).toContainEqual({
      url: "https://store.example/api/stores",
      method: "POST",
      body: JSON.stringify({ name: "design", source: "git@example.com:design.git" }),
      authorization: "Bearer store-token",
    });

    setActiveDaemon({ id: "other", label: "Other", baseURL: "https://other.example", token: "other-token" });
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("refresh failed");
    expect(screen.getByRole("heading", { name: "design" })).toBeInTheDocument();
    expect(calls).toContainEqual(expect.objectContaining({
      url: "https://store.example/api/stores/design/refresh",
      method: "POST",
      authorization: "Bearer store-token",
    }));

    fireEvent.click(screen.getByRole("button", { name: "Build reviewer" }));
    await waitFor(() => expect(calls).toContainEqual({
      url: "https://store.example/api/images/build",
      method: "POST",
      body: JSON.stringify({ source: "design/reviewer" }),
      authorization: "Bearer store-token",
    }));
    expect(screen.getByRole("button", { name: "Building reviewer…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Refresh" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Remove store" })).toBeDisabled();

    finishBuild(envelope({ name: "reviewer", tag: "1.2.3", digest: "sha256:new", layers: 2 }));
    expect(await screen.findByText("Built reviewer:1.2.3.")).toBeInTheDocument();
    expect(imageBuilt).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: "Remove store" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText(/preserves local sources and built images/i)).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove store" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("remove failed");
    expect(within(dialog).getByRole("button", { name: "Remove store" })).toBeEnabled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove store" }));
    await waitFor(() => expect(calls).toContainEqual(expect.objectContaining({
      url: "https://store.example/api/stores/design",
      method: "DELETE",
      authorization: "Bearer store-token",
    })));
    expect(await screen.findByRole("heading", { name: "Stores" })).toBeInTheDocument();

    window.removeEventListener("tariboy:image-built", imageBuilt);
  });

  it("discards a detail response after the route selects another Store", async () => {
    const host = await addDaemon({
      label: "Store host",
      baseURL: "https://stale.example",
      token: "stale-token",
    });
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
      { host: { id: host.id, label: host.label }, agents: [] },
    ]);
    let finishOld!: (response: Response) => void;
    const oldResponse = new Promise<Response>((resolve) => { finishOld = resolve; });
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/api/stores/old")) return oldResponse;
      if (url.endsWith("/api/stores/new")) return Promise.resolve(envelope({
        name: "new", source: "/srv/new", path: "/srv/new", images: [],
      }));
      return Promise.resolve(envelope({ agents: [], groups: [], count: 0 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    const hostParam = encodeURIComponent(host.id);
    renderPage(`/servers/${hostParam}/stores/old`, hostParam);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "https://stale.example/api/stores/old",
      expect.any(Object),
    ));
    fireEvent.click(screen.getByRole("button", { name: "Open new" }));
    expect(await screen.findByRole("heading", { name: "new" })).toBeInTheDocument();
    finishOld(envelope({ name: "old", source: "/srv/old", path: "/srv/old", images: [] }));

    await waitFor(() => expect(screen.queryByRole("heading", { name: "old" })).not.toBeInTheDocument());
    expect(screen.getByRole("heading", { name: "new" })).toBeInTheDocument();
  });
});
