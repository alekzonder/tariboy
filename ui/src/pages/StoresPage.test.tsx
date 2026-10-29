import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router-dom";
import { ApiProvider } from "@/components/ApiProvider";
import { DaemonProvider } from "@/components/DaemonProvider";
import { CustomerQuestionNotificationsContext } from "@/components/customerQuestionNotificationsContext";
import { SidebarStateProvider } from "@/pages/terminals/SidebarStateProvider";
import TerminalsPage from "@/pages/terminals/TerminalsPage";
import { fetchAllAgents } from "@/lib/aggregate";
import { setActiveDaemon } from "@/lib/api";
import { addDaemon } from "@/lib/daemons";
import { subscribeImagesChanged } from "@/lib/imageEvents";
import StoresPage from "@/pages/StoresPage";
import { routeFetch } from "@/test/fetchRoutes";
import { FakeWebSocket } from "@/test/fakeWebSocket";
import { operationStarted } from "@/test/fixtures/apiFixtures";

vi.mock("@/lib/aggregate", () => ({ fetchAllAgents: vi.fn() }));

interface Call {
  url: string;
  method: string;
  body?: string;
  authorization?: string;
}

const envelope = (result: unknown, ok = true): Response =>
  ({
    ok,
    status: ok ? 200 : 500,
    text: async () =>
      JSON.stringify(
        ok
          ? { ok: true, result }
          : { ok: false, error: { code: "store_failed", message: String(result) } },
      ),
  }) as Response;

function RouteButtons({
  hostParam,
  otherHostParam,
}: {
  hostParam: string;
  otherHostParam?: string;
}) {
  const navigate = useNavigate();
  return (
    <>
      <button onClick={() => navigate(`/servers/${hostParam}/stores/old`)}>Open old</button>
      <button onClick={() => navigate(`/servers/${otherHostParam ?? hostParam}/stores/new`)}>
        Open new
      </button>
      {otherHostParam && (
        <button onClick={() => navigate(`/servers/${otherHostParam}/stores/old`)}>
          Switch host
        </button>
      )}
    </>
  );
}

function renderPage(path: string, hostParam: string, otherHostParam?: string) {
  return render(
    <DaemonProvider>
      <CustomerQuestionNotificationsContext.Provider
        value={{ attention: new Map(), refreshHost: async () => {} }}
      >
        <MemoryRouter initialEntries={[path]}>
          <SidebarStateProvider>
            <RouteButtons hostParam={hostParam} otherHostParam={otherHostParam} />
            <Routes>
              <Route
                path="/servers/:hostId/stores"
                element={<TerminalsPage serverView="stores" />}
              />
              <Route
                path="/servers/:hostId/stores/:name"
                element={<TerminalsPage serverView="store-detail" />}
              />
            </Routes>
          </SidebarStateProvider>
        </MemoryRouter>
      </CustomerQuestionNotificationsContext.Provider>
    </DaemonProvider>,
  );
}

beforeEach(() => {
  localStorage.clear();
  // The server rows this page asserts on live in the sidebar's Servers tab.
  localStorage.setItem("terminals:sidebar-tab:v1", "servers");
  sessionStorage.clear();
  FakeWebSocket.install();
});

afterEach(() => {
  setActiveDaemon(null);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
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
    let detailReads = 0;
    let detailReloadFails = false;
    let finishBuild!: (response: Response) => void;
    const buildResponse = new Promise<Response>((resolve) => {
      finishBuild = resolve;
    });
    const imageBuilt = vi.fn();
    const stopImageBuilt = subscribeImagesChanged(imageBuilt);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
        const headers = new Headers(init.headers);
        calls.push({
          url: String(input),
          method: init.method ?? "GET",
          body: typeof init.body === "string" ? init.body : undefined,
          authorization: headers.get("Authorization") ?? undefined,
        });
        return storeRoutes(input, init);
      }),
    );
    const storeRoutes = routeFetch(
      {
        "GET /api/stores": () =>
          envelope([{ name: "team", source: "/srv/team", path: "/srv/team" }]),
        "POST /api/stores": () =>
          envelope({
            name: "design",
            source: "git@example.com:design.git",
            path: "/stores/design",
          }),
        "GET /api/stores/design": () => {
          if (detailReloadFails) return envelope("reload failed", false);
          detailReads++;
          return envelope({
            name: "design",
            source: "git@example.com:design.git",
            path: "/stores/design",
            images: [
              {
                name: "reviewer",
                version: "1.2.3",
                built_version: detailReads === 1 ? "1.0.0" : "1.2.3",
                update_needed: detailReads === 1,
                latest_status: "built",
              },
              {
                name: "equal",
                version: "1.0.0",
                built_version: "1.0.0",
                update_needed: false,
                latest_status: "built",
              },
              {
                name: "missing-latest",
                version: "2.0.0",
                built_version: "",
                update_needed: false,
                latest_status: "missing",
              },
              {
                name: "source-unversioned",
                version: "",
                built_version: "1.0.0",
                update_needed: false,
                latest_status: "built",
              },
              {
                name: "latest-unversioned",
                version: "1.0.0",
                built_version: "",
                update_needed: false,
                latest_status: "unversioned",
              },
              {
                name: "latest-broken",
                version: "1.0.0",
                built_version: "",
                update_needed: false,
                latest_status: "error",
                latest_error: "invalid latest manifest",
              },
              {
                name: "broken",
                version: "",
                built_version: "",
                update_needed: false,
                error: "invalid Tariboyfile",
                latest_status: "missing",
              },
            ],
          });
        },
        "* /api/stores/design/refresh": () => envelope("refresh failed", false),
        "* /api/images/build": () => buildResponse,
        "DELETE /api/stores/design": () => {
          removeAttempts++;
          return removeAttempts === 1
            ? envelope("remove failed", false)
            : envelope({ removed: true });
        },
      },
      () => envelope({ agents: [], groups: [], count: 0 }),
    );

    renderPage(`/servers/${encodeURIComponent(host.id)}/stores`, encodeURIComponent(host.id));

    expect(await screen.findByRole("heading", { name: "Stores" })).toBeInTheDocument();
    expect(await screen.findByText("/srv/team")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Images" })).toHaveAttribute(
      "href",
      `/servers/${encodeURIComponent(host.id)}/images`,
    );
    expect(screen.getByRole("link", { name: "Stores" })).toHaveAttribute(
      "href",
      `/servers/${encodeURIComponent(host.id)}/stores`,
    );

    fireEvent.change(screen.getByLabelText("Store name"), { target: { value: "design" } });
    fireEvent.change(screen.getByLabelText("Store source"), {
      target: { value: "git@example.com:design.git" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add store" }));

    expect(await screen.findByRole("heading", { name: "design" })).toBeInTheDocument();
    expect(
      await screen.findByRole("columnheader", { name: "Source image_version" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Latest image_version" })).toBeInTheDocument();
    const reviewerRow = screen.getByText("reviewer").closest("tr")!;
    expect(reviewerRow).toHaveTextContent("1.0.0");
    expect(reviewerRow).toHaveTextContent("Update needed");
    expect(reviewerRow).toHaveClass("bg-amber-50");
    const equalRow = screen.getByText("equal").closest("tr")!;
    expect(equalRow).toHaveTextContent("Up to date");
    expect(within(equalRow).getByRole("button", { name: "Build equal" })).toBeEnabled();
    const missingLatestRow = screen.getByText("missing-latest").closest("tr")!;
    expect(missingLatestRow).toHaveTextContent("Not built");
    expect(missingLatestRow).toHaveTextContent("Latest not built");
    expect(
      within(missingLatestRow).getByRole("button", { name: "Build missing-latest" }),
    ).toBeEnabled();
    const sourceUnversionedRow = screen.getByText("source-unversioned").closest("tr")!;
    expect(sourceUnversionedRow).toHaveTextContent(
      "Source image_version missing; comparison unavailable",
    );
    expect(sourceUnversionedRow).not.toHaveTextContent("Up to date");
    const latestUnversionedRow = screen.getByText("latest-unversioned").closest("tr")!;
    expect(latestUnversionedRow).toHaveTextContent(
      "Latest image_version missing; comparison unavailable",
    );
    expect(latestUnversionedRow).not.toHaveTextContent("Up to date");
    const latestBrokenRow = screen.getByText("latest-broken").closest("tr")!;
    expect(latestBrokenRow).toHaveTextContent("Latest error: invalid latest manifest");
    expect(
      within(latestBrokenRow).getByRole("button", { name: "Build latest-broken" }),
    ).toBeEnabled();
    const brokenRow = screen.getByText("broken").closest("tr")!;
    expect(brokenRow).toHaveTextContent("Source error: invalid Tariboyfile");
    expect(brokenRow).toHaveTextContent("Latest not built");
    expect(within(brokenRow).getByRole("button", { name: "Build broken" })).toBeDisabled();
    expect(calls).toContainEqual({
      url: "https://store.example/api/stores",
      method: "POST",
      body: JSON.stringify({ name: "design", source: "git@example.com:design.git" }),
      authorization: "Bearer store-token",
    });

    setActiveDaemon({
      id: "other",
      label: "Other",
      baseURL: "https://other.example",
      token: "other-token",
    });
    const refresh = screen.getByRole("button", { name: "Refresh" });
    const removeStore = screen.getByRole("button", { name: "Remove store" });
    fireEvent.click(refresh);
    expect(await screen.findByRole("alert")).toHaveTextContent("refresh failed");
    expect(screen.getByRole("heading", { name: "design" })).toBeInTheDocument();
    expect(calls).toContainEqual(
      expect.objectContaining({
        url: "https://store.example/api/stores/design/refresh",
        method: "POST",
        authorization: "Bearer store-token",
      }),
    );

    fireEvent.click(within(reviewerRow).getByRole("button", { name: "Build reviewer" }));
    await waitFor(() =>
      expect(calls).toContainEqual({
        url: "https://store.example/api/images/build",
        method: "POST",
        body: JSON.stringify({ source: "design/reviewer", initiator: "ui" }),
        authorization: "Bearer store-token",
      }),
    );
    expect(within(reviewerRow).getByRole("button", { name: "Building reviewer…" })).toBeDisabled();
    expect(refresh).toBeDisabled();
    expect(removeStore).toBeDisabled();

    finishBuild(envelope(operationStarted({ id: 11, title: "Build image design/reviewer" })));
    expect(
      await screen.findByText("Build started: Build image design/reviewer."),
    ).toBeInTheDocument();
    // The build runs on: the store reloads only when its operation succeeds.
    expect(detailReads).toBe(1);
    expect(imageBuilt).not.toHaveBeenCalled();
    (await FakeWebSocket.dialed("/api/operations/ws")).send({
      id: 11,
      status: "succeeded",
      ts: "t",
    });
    await waitFor(() => expect(detailReads).toBe(2));
    expect(screen.getByText("reviewer").closest("tr")).not.toHaveClass("bg-amber-50");
    expect(screen.getByText("reviewer").closest("tr")).toHaveTextContent("Up to date");
    expect(
      within(screen.getByText("reviewer").closest("tr")!).getByRole("button", {
        name: "Build reviewer",
      }),
    ).toBeEnabled();
    expect(imageBuilt).toHaveBeenCalledOnce();

    detailReloadFails = true;
    fireEvent.click(
      within(screen.getByText("reviewer").closest("tr")!).getByRole("button", {
        name: "Build reviewer",
      }),
    );
    await waitFor(() =>
      expect(
        within(screen.getByText("reviewer").closest("tr")!).getByRole("button", {
          name: "Build reviewer",
        }),
      ).toBeEnabled(),
    );
    // The first build ended, so its socket closed; the second build dials again.
    await waitFor(() =>
      expect(
        FakeWebSocket.instances.filter((socket) => socket.url.includes("/api/operations/ws")),
      ).toHaveLength(2),
    );
    (await FakeWebSocket.dialed("/api/operations/ws")).send({
      id: 11,
      status: "succeeded",
      ts: "t",
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("reload failed");
    expect(screen.getByText("Build started: Build image design/reviewer.")).toBeInTheDocument();
    expect(imageBuilt).toHaveBeenCalledTimes(2);

    fireEvent.click(removeStore);
    const dialog = await screen.findByRole("alertdialog");
    expect(
      within(dialog).getByText(/preserves local sources and built images/i),
    ).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove store" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("remove failed");
    expect(within(dialog).getByRole("button", { name: "Remove store" })).toBeEnabled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove store" }));
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          url: "https://store.example/api/stores/design",
          method: "DELETE",
          authorization: "Bearer store-token",
        }),
      ),
    );
    expect(await screen.findByRole("heading", { name: "Stores" })).toBeInTheDocument();

    stopImageBuilt();
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
    const oldResponse = new Promise<Response>((resolve) => {
      finishOld = resolve;
    });
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/api/stores/old")) return oldResponse;
      if (url.endsWith("/api/stores/new"))
        return Promise.resolve(
          envelope({
            name: "new",
            source: "/srv/new",
            path: "/srv/new",
            images: [],
          }),
        );
      return Promise.resolve(envelope({ agents: [], groups: [], count: 0 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    const hostParam = encodeURIComponent(host.id);
    renderPage(`/servers/${hostParam}/stores/old`, hostParam);

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "https://stale.example/api/stores/old",
        expect.any(Object),
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Open new" }));
    expect(await screen.findByRole("heading", { name: "new" })).toBeInTheDocument();
    finishOld(envelope({ name: "old", source: "/srv/old", path: "/srv/old", images: [] }));

    await waitFor(() =>
      expect(screen.queryByRole("heading", { name: "old" })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("heading", { name: "new" })).toBeInTheDocument();
  });

  it("isolates late builds by host and supports an explicit target ref", async () => {
    const host = await addDaemon({
      label: "Old Store host",
      baseURL: "https://old-build.example",
      token: "old-build-token",
    });
    const newHost = await addDaemon({
      label: "New Store host",
      baseURL: "https://new-build.example",
      token: "new-build-token",
    });
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
      { host: { id: host.id, label: host.label }, agents: [] },
      { host: { id: newHost.id, label: newHost.label }, agents: [] },
    ]);
    let finishOld!: (response: Response) => void;
    let finishNewFailure!: (response: Response) => void;
    let finishNewSuccess!: (response: Response) => void;
    let finishThirdFailure!: (response: Response) => void;
    let finishNewReload!: (response: Response) => void;
    const oldBuild = new Promise<Response>((resolve) => {
      finishOld = resolve;
    });
    const newFailure = new Promise<Response>((resolve) => {
      finishNewFailure = resolve;
    });
    const newSuccess = new Promise<Response>((resolve) => {
      finishNewSuccess = resolve;
    });
    const thirdFailure = new Promise<Response>((resolve) => {
      finishThirdFailure = resolve;
    });
    const newReload = new Promise<Response>((resolve) => {
      finishNewReload = resolve;
    });
    let newAttempts = 0;
    let newReads = 0;
    const calls: Call[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
        const url = String(input);
        const method = init.method ?? "GET";
        const body = typeof init.body === "string" ? init.body : undefined;
        calls.push({ url, method, body });
        if (url.startsWith("https://old-build.example") && url.endsWith("/api/stores/old"))
          return Promise.resolve(
            envelope({
              name: "old",
              source: "/srv/old",
              path: "/srv/old",
              images: [
                {
                  name: "old-image",
                  version: "1.0.0",
                  built_version: "",
                  update_needed: false,
                  latest_status: "missing",
                },
              ],
            }),
          );
        if (url.startsWith("https://new-build.example") && url.endsWith("/api/stores/old")) {
          newReads++;
          if (newReads > 1) return newReload;
          return Promise.resolve(
            envelope({
              name: "old",
              source: "/srv/new",
              path: "/srv/new",
              images: [
                {
                  name: "new-image",
                  version: "2.0.0",
                  built_version: "",
                  update_needed: false,
                  latest_status: "missing",
                },
              ],
            }),
          );
        }
        if (url.endsWith("/api/images/build")) {
          const source = JSON.parse(body ?? "{}").source;
          if (source === "old/old-image") return oldBuild;
          newAttempts++;
          return newAttempts === 1 ? newFailure : newAttempts === 2 ? newSuccess : thirdFailure;
        }
        return Promise.resolve(envelope({ agents: [], groups: [], count: 0 }));
      }),
    );
    const hostParam = encodeURIComponent(host.id);
    const newHostParam = encodeURIComponent(newHost.id);
    renderPage(`/servers/${hostParam}/stores/old`, hostParam, newHostParam);

    fireEvent.click(await screen.findByRole("button", { name: "Build old-image" }));
    expect(screen.getByRole("button", { name: "Building old-image…" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Switch host" }));
    expect(await screen.findByRole("button", { name: "Build new-image" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Open server New Store host" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(await screen.findByRole("button", { name: "Build new-image" })).toBeEnabled();

    expect(screen.queryByLabelText("Target image name")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Target image tag")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Build new-image" }));
    expect(screen.getByRole("button", { name: "Building new-image…" })).toBeDisabled();
    expect(calls).toContainEqual(
      expect.objectContaining({
        url: "https://new-build.example/api/images/build",
        method: "POST",
        body: JSON.stringify({ source: "old/new-image", initiator: "ui" }),
      }),
    );

    finishOld(envelope(operationStarted({ id: 1, title: "Build image old/old-image" })));
    await waitFor(() => expect(screen.queryByText(/old\/old-image/)).not.toBeInTheDocument());
    expect(screen.getByRole("heading", { name: "old" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Building new-image…" })).toBeDisabled();

    finishNewFailure(envelope("immutable target", false));
    expect(await screen.findByRole("alert")).toHaveTextContent("immutable target");
    expect(screen.getByRole("button", { name: "Build new-image" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Build new-image" }));
    finishNewSuccess(envelope(operationStarted({ id: 2, title: "Build image old/new-image" })));
    expect(
      await screen.findByText("Build started: Build image old/new-image."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Build new-image" })).toBeEnabled();
    // The build succeeds and its reload starts; a newer build makes that reload late.
    (await FakeWebSocket.dialed("/api/operations/ws")).send({
      id: 2,
      status: "succeeded",
      ts: "t",
    });
    await waitFor(() => expect(newReads).toBe(2));
    fireEvent.click(screen.getByRole("button", { name: "Build new-image" }));
    finishNewReload(envelope({ name: "old", source: "/srv/new", path: "/srv/new", images: [] }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Building new-image…" })).toBeDisabled(),
    );
    expect(screen.getByText("new-image")).toBeInTheDocument();
    finishThirdFailure(envelope("third build failed", false));
    expect(await screen.findByRole("alert")).toHaveTextContent("third build failed");
    expect(screen.getByRole("button", { name: "Build new-image" })).toBeEnabled();
  });

  it("keeps Refresh ownership when build inventory finishes late", async () => {
    const host = await addDaemon({
      label: "Store host",
      baseURL: "https://refresh-build.example",
      token: "refresh-build-token",
    });
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
      { host: { id: host.id, label: host.label }, agents: [] },
    ]);
    let finishStaleRead!: (response: Response) => void;
    let finishRefresh!: (response: Response) => void;
    const staleRead = new Promise<Response>((resolve) => {
      finishStaleRead = resolve;
    });
    const refreshResponse = new Promise<Response>((resolve) => {
      finishRefresh = resolve;
    });
    let detailReads = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
        const url = String(input);
        const method = init.method ?? "GET";
        if (url.endsWith("/api/stores/team") && method === "GET") {
          detailReads++;
          if (detailReads > 1) return staleRead;
          return Promise.resolve(
            envelope({
              name: "team",
              source: "/srv/team",
              path: "/srv/team",
              images: [
                {
                  name: "reviewer",
                  version: "1.0.0",
                  built_version: "",
                  update_needed: false,
                  latest_status: "missing",
                },
              ],
            }),
          );
        }
        if (url.endsWith("/api/images/build")) {
          return Promise.resolve(
            envelope(operationStarted({ id: 5, title: "Build image team/reviewer" })),
          );
        }
        if (url.endsWith("/api/stores/team/refresh")) return refreshResponse;
        return Promise.resolve(envelope({ agents: [], groups: [], count: 0 }));
      }),
    );
    const hostParam = encodeURIComponent(host.id);
    renderPage(`/servers/${hostParam}/stores/team`, hostParam);

    fireEvent.click(await screen.findByRole("button", { name: "Build reviewer" }));
    expect(
      await screen.findByText("Build started: Build image team/reviewer."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Build reviewer" })).toBeEnabled();
    (await FakeWebSocket.dialed("/api/operations/ws")).send({
      id: 5,
      status: "succeeded",
      ts: "t",
    });
    await waitFor(() => expect(detailReads).toBe(2));
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(screen.getByRole("button", { name: "Refreshing…" })).toBeDisabled();

    finishStaleRead(envelope("stale inventory failed", false));
    await waitFor(() =>
      expect(screen.queryByText("stale inventory failed")).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: "Refreshing…" })).toBeDisabled();

    finishRefresh(
      envelope({
        name: "team",
        source: "/srv/team",
        path: "/srv/team",
        images: [
          {
            name: "fresh",
            version: "2.0.0",
            built_version: "2.0.0",
            update_needed: false,
            latest_status: "built",
          },
        ],
      }),
    );
    expect(await screen.findByText("Refreshed team.")).toBeInTheDocument();
    expect(screen.getByText("fresh")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Refresh" })).toBeEnabled();
  });
});

describe("Store automatic builds", () => {
  it("loads the saved policy and posts the edited interval and image selection", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
    ]);
    const calls: Call[] = [];
    const images = [
      {
        name: "reviewer",
        version: "1.2.3",
        built_version: "1.0.0",
        update_needed: true,
        latest_status: "built",
      },
      {
        name: "writer",
        version: "1.0.0",
        built_version: "1.0.0",
        update_needed: false,
        latest_status: "built",
      },
    ];
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
        const url = String(input);
        const method = init.method ?? "GET";
        calls.push({ url, method, body: typeof init.body === "string" ? init.body : undefined });
        if (url.endsWith("/api/stores/old/auto") && method === "POST") {
          return Promise.resolve(
            envelope({
              name: "old",
              source: "/srv/old",
              path: "/srv/old",
              images,
              auto: { interval_minutes: 30, images: ["reviewer", "writer"] },
            }),
          );
        }
        if (url.endsWith("/api/stores/old") && method === "GET") {
          return Promise.resolve(
            envelope({
              name: "old",
              source: "/srv/old",
              path: "/srv/old",
              images,
              auto: { interval_minutes: 15, images: ["reviewer"] },
            }),
          );
        }
        return Promise.resolve(envelope({ agents: [], groups: [], count: 0 }));
      }),
    );

    renderPage("/servers/local/stores/old", "local");

    const interval = await screen.findByLabelText("Automatic build interval");
    await waitFor(() => expect(interval).toHaveValue(15));
    expect(screen.getByLabelText("Automatic builds for reviewer")).toBeChecked();
    expect(screen.getByLabelText("Automatic builds for writer")).not.toBeChecked();

    fireEvent.click(screen.getByLabelText("Automatic builds for writer"));
    fireEvent.change(interval, { target: { value: "30" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automatic builds" }));

    await waitFor(() =>
      expect(calls.some((call) => call.url.endsWith("/api/stores/old/auto"))).toBe(true),
    );
    const saved = calls.find((call) => call.url.endsWith("/api/stores/old/auto"));
    expect(saved?.method).toBe("POST");
    expect(JSON.parse(saved?.body ?? "{}")).toEqual({
      interval: 30,
      image: ["reviewer", "writer"],
    });
    expect(
      await screen.findByText("Automatic builds run every 30 min for 2 image(s)."),
    ).toBeInTheDocument();
  });
});

describe("Store workflow images", () => {
  it("lists workflow sources in their own tab and builds one from the Store", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
    ]);
    const calls: Call[] = [];
    let built = false;
    let buildFails = true;
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
        const url = String(input);
        const method = init.method ?? "GET";
        calls.push({ url, method, body: typeof init.body === "string" ? init.body : undefined });
        if (url.endsWith("/api/stores/old") && method === "GET") {
          return Promise.resolve(
            envelope({
              name: "old",
              source: "/srv/old",
              path: "/srv/old",
              images: [
                {
                  name: "reviewer",
                  version: "1.0.0",
                  built_version: "1.0.0",
                  update_needed: false,
                  latest_status: "built",
                },
              ],
              workflows: [
                { name: "development", version: "0.3.0" },
                { name: "equal", version: "1.0.0" },
                { name: "fresh", version: "0.1.0" },
                { name: "broken", version: "", error: "invalid Workflowfile.yaml" },
              ],
            }),
          );
        }
        if (url.endsWith("/api/workflow-images") && method === "GET") {
          return Promise.resolve(
            envelope({
              workflows: [
                {
                  name: "development",
                  tag: "latest",
                  version: built ? "0.3.0" : "0.2.0",
                  digest: "d1",
                  built_at: "",
                },
                { name: "development", tag: "0.2.0", version: "0.2.0", digest: "d1", built_at: "" },
                { name: "equal", tag: "latest", version: "1.0.0", digest: "d2", built_at: "" },
              ],
              count: 3,
            }),
          );
        }
        if (url.endsWith("/api/workflow-images/build") && method === "POST") {
          if (buildFails) {
            buildFails = false;
            return Promise.resolve(envelope("workflow build failed", false));
          }
          built = true;
          return Promise.resolve(
            envelope(
              operationStarted({
                id: 9,
                kind: "workflow.build",
                title: "Build workflow old/development",
              }),
            ),
          );
        }
        return Promise.resolve(envelope({ agents: [], groups: [], count: 0 }));
      }),
    );

    renderPage("/servers/local/stores/old", "local");

    expect(await screen.findByRole("button", { name: "Build reviewer" })).toBeInTheDocument();
    fireEvent.mouseDown(screen.getByRole("tab", { name: "Workflow images" }));
    const row = async (workflow: string) =>
      (await screen.findByText(workflow, { selector: "td" })).closest("tr") as HTMLElement;
    expect(screen.queryByRole("button", { name: "Build reviewer" })).not.toBeInTheDocument();
    await waitFor(async () =>
      expect(within(await row("development")).getByText("Update needed")).toBeInTheDocument(),
    );
    expect(within(await row("development")).getByText("0.2.0")).toBeInTheDocument();
    expect(within(await row("equal")).getByText("Up to date")).toBeInTheDocument();
    expect(within(await row("fresh")).getByText("Latest not built")).toBeInTheDocument();
    expect(
      within(await row("broken")).getByText("Source error: invalid Workflowfile.yaml"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Build broken" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Build development" }));
    expect(await screen.findByText("workflow build failed")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Build development" }));
    expect(
      await screen.findByText("Build started: Build workflow old/development."),
    ).toBeInTheDocument();
    const build = calls.filter((call) => call.url.endsWith("/api/workflow-images/build"));
    expect(JSON.parse(build[1]?.body ?? "{}")).toEqual({
      source: "old/development",
      initiator: "ui",
    });
    (await FakeWebSocket.dialed("/api/operations/ws")).send({
      id: 9,
      status: "succeeded",
      ts: "t",
    });
    await waitFor(async () =>
      expect(within(await row("development")).getByText("Up to date")).toBeInTheDocument(),
    );
  });

  it("keeps workflow builds available when built images cannot be compared", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
    ]);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
        const url = String(input);
        const method = init.method ?? "GET";
        if (url.endsWith("/api/stores/old") && method === "GET") {
          return Promise.resolve(
            envelope({
              name: "old",
              source: "/srv/old",
              path: "/srv/old",
              images: [],
              workflows: [{ name: "development", version: "0.3.0" }],
            }),
          );
        }
        if (url.endsWith("/api/workflow-images"))
          return Promise.resolve(envelope("registry down", false));
        return Promise.resolve(envelope({ agents: [], groups: [], count: 0 }));
      }),
    );

    renderPage("/servers/local/stores/old", "local");

    fireEvent.mouseDown(await screen.findByRole("tab", { name: "Workflow images" }));
    expect(await screen.findByText("Comparison unavailable")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Build development" })).toBeEnabled();
  });
});

describe("Store reconnect", () => {
  it("keeps the edited policy while the host reconnects on a new endpoint", async () => {
    const calls: Call[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
        calls.push({ url: String(input), method: init.method ?? "GET" });
        return Promise.resolve(
          envelope(
            String(input).includes("/api/stores/old")
              ? {
                  name: "old",
                  source: "/srv/old",
                  path: "/srv/old",
                  images: [],
                  auto: { interval_minutes: 15, images: [] },
                }
              : [],
          ),
        );
      }),
    );
    const page = (baseURL: string) => (
      <MemoryRouter>
        <ApiProvider target={{ id: "remote", label: "Remote", baseURL, token: "t" }}>
          <StoresPage name="old" basePath="/servers/remote/stores" />
        </ApiProvider>
      </MemoryRouter>
    );
    const view = render(page("http://127.0.0.1:41001"));
    const interval = await screen.findByLabelText("Automatic build interval");
    await waitFor(() => expect(interval).toHaveValue(15));
    fireEvent.change(interval, { target: { value: "45" } });

    view.rerender(page(""));
    view.rerender(page("http://127.0.0.1:41002"));
    await waitFor(() =>
      expect(
        calls.some((call) => call.url.startsWith("http://127.0.0.1:41002/api/stores/old")),
      ).toBe(true),
    );

    expect(screen.getByLabelText("Automatic build interval")).toHaveValue(45);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("Tariboy Store button", () => {
  const tariboyStore = {
    name: "tariboy",
    source: "https://github.com/alekzonder/tariboy-store.git",
  };
  const target = {
    id: "remote",
    label: "Remote",
    baseURL: "https://store.example",
    token: "store-token",
  };

  const renderList = () =>
    render(
      <MemoryRouter initialEntries={["/servers/remote/stores"]}>
        <Routes>
          <Route
            path="/servers/remote/stores"
            element={
              <ApiProvider target={target}>
                <StoresPage basePath="/servers/remote/stores" />
              </ApiProvider>
            }
          />
          <Route path="/servers/remote/stores/:name" element={<h1>Opened Store detail</h1>} />
        </Routes>
      </MemoryRouter>,
    );

  const stubStores = (stores: unknown[], post: () => Response) => {
    const calls: Call[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((input: RequestInfo | URL, init: RequestInit = {}) => {
        const method = init.method ?? "GET";
        calls.push({
          url: String(input),
          method,
          body: typeof init.body === "string" ? init.body : undefined,
        });
        return Promise.resolve(method === "POST" ? post() : envelope(stores));
      }),
    );
    return calls;
  };

  it("adds the tariboy Store with one click and opens it", async () => {
    const calls = stubStores([], () => envelope({ ...tariboyStore, path: "/stores/tariboy" }));
    renderList();

    fireEvent.click(await screen.findByRole("button", { name: "Add tariboy Store" }));

    expect(await screen.findByRole("heading", { name: "Opened Store detail" })).toBeInTheDocument();
    expect(calls).toContainEqual({
      url: "https://store.example/api/stores",
      method: "POST",
      body: JSON.stringify(tariboyStore),
    });
  });

  it("shows the daemon error and enables the button again", async () => {
    stubStores([], () => envelope("clone Store: git failed", false));
    renderList();

    fireEvent.click(await screen.findByRole("button", { name: "Add tariboy Store" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("clone Store: git failed");
    expect(screen.getByRole("button", { name: "Add tariboy Store" })).toBeEnabled();
  });

  it.each([
    ["the name is taken", { name: "tariboy", source: "/srv/other" }],
    ["the source is registered", { name: "official", source: tariboyStore.source }],
  ])("is hidden when %s", async (_case, store) => {
    stubStores([{ ...store, path: "/stores/x" }], () => envelope({}));
    renderList();

    expect(await screen.findByText(store.source)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add tariboy Store" })).toBeNull();
  });
});
