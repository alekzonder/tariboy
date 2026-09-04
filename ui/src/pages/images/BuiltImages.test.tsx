import { afterEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { useLayoutEffect, useState } from "react";
import { addDaemon, updateDaemon, type Daemon, type DaemonMeta } from "@/lib/daemons";
import * as daemonsApi from "@/lib/daemons";
import { setActiveDaemon } from "@/lib/api";
import * as imageTransferApi from "@/lib/teamApi";
import BuiltImages from "./BuiltImages";

const daemonContext = vi.hoisted(() => ({ daemons: [] as DaemonMeta[] }));

vi.mock("@/components/DaemonProvider", () => ({
  useOptionalDaemons: () => ({ daemons: daemonContext.daemons }),
}));

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  daemonContext.daemons = [];
  setActiveDaemon(null);
});

it("starts transfer from the route source instead of the active daemon", async () => {
  const source = await addDaemon({ label: "Source", baseURL: "https://source", token: "source-token" });
  const target = await addDaemon({ label: "Target", baseURL: "https://target", token: "target-token" });
  daemonContext.daemons = [{ ...source, state: "ready" }, { ...target, state: "ready" }];
  await updateDaemon(target.id, { label: "Target", baseURL: "https://target-reconnected" });
  const activeTarget = { id: "active", label: "Active", baseURL: "https://active", token: "active-token" };
  setActiveDaemon(activeTarget);
  const download = vi.spyOn(imageTransferApi, "downloadImageArchiveOn").mockResolvedValue(new Blob(["archive"]));
  vi.spyOn(imageTransferApi, "uploadImageArchiveOn").mockResolvedValue({ import_id: "import-1", ref: "built:v1", digest: "digest" });
  vi.spyOn(imageTransferApi, "applyImageArchiveOn").mockResolvedValue({});
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL) => {
    if (String(input) === "https://source/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [
        { name: "built", tag: "v1", bare: false, exportable: true },
        { name: "basic", tag: "latest", bare: false, exportable: false },
      ] } }), { status: 200 }));
    }
    return Promise.reject(new Error(`unexpected request ${String(input)}`));
  }));

  render(<MemoryRouter><BuiltImages hostId={source.id} /></MemoryRouter>);

  await screen.findByRole("button", { name: "Upload to servers built:v1" });
  expect(screen.queryByRole("button", { name: /Upload to servers basic:latest/ })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Upload to servers built:v1" }));
  fireEvent.click(await screen.findByRole("button", { name: "All servers" }));
  fireEvent.click(screen.getByRole("button", { name: "Start transfer" }));

  await waitFor(() => expect(download).toHaveBeenCalledWith(
    expect.objectContaining({ id: source.id, baseURL: "https://source" }),
    "built:v1",
  ));
  expect(download).not.toHaveBeenCalledWith(activeTarget, "built:v1");
  await waitFor(() => expect(imageTransferApi.uploadImageArchiveOn).toHaveBeenCalledWith(
    expect.objectContaining({ id: target.id, baseURL: "https://target-reconnected" }),
    expect.any(Blob),
  ));
});

it("exports from the route host instead of the active daemon", async () => {
  const source = await addDaemon({ label: "Source", baseURL: "https://source", token: "source-token" });
  setActiveDaemon({ id: "active", label: "Active", baseURL: "https://active", token: "active-token" });
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const path = String(input);
    if (path === "https://source/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [
        { name: "reviewer", tag: "v3", bare: false, exportable: true },
      ] } }), { status: 200 }));
    }
    if (path === "https://source/api/images/reviewer%3Av3/export") {
      return Promise.resolve({ ok: true, status: 200, blob: () => Promise.resolve(new Blob(["archive"])) } as Response);
    }
    return Promise.reject(new Error(`unexpected request ${path}`));
  });
  vi.stubGlobal("fetch", fetchMock);
  vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:archive");
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
  let downloaded = "";
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) { downloaded = this.download; });

  render(<MemoryRouter><BuiltImages hostId={source.id} /></MemoryRouter>);

  fireEvent.click(await screen.findByRole("button", { name: "Export reviewer:v3" }));
  await waitFor(() => expect(downloaded).toBe("reviewer-v3.tariboy-image.tar.gz"));
  expect(fetchMock).toHaveBeenCalledWith(
    "https://source/api/images/reviewer%3Av3/export",
    expect.objectContaining({ method: "GET" }),
  );
});

it("imports into the route host instead of the active daemon", async () => {
  const source = await addDaemon({ label: "Source", baseURL: "https://source", token: "source-token" });
  setActiveDaemon({ id: "active", label: "Active", baseURL: "https://active", token: "active-token" });
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path === "https://source/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [] } }), { status: 200 }));
    }
    if (path === "https://source/api/image-imports" && init?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { import_id: "import-1", ref: "reviewer:v1", digest: "abc" } }), { status: 200 }));
    }
    if (path === "https://source/api/image-imports/import-1/apply" && init?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: {} }), { status: 200 }));
    }
    return Promise.reject(new Error(`unexpected request ${init?.method} ${path}`));
  });
  vi.stubGlobal("fetch", fetchMock);

  render(<MemoryRouter><BuiltImages hostId={source.id} /></MemoryRouter>);

  const archiveInput = await screen.findByLabelText("Import image archive");
  await waitFor(() => expect(archiveInput).toBeEnabled());
  fireEvent.change(archiveInput, {
    target: { files: [new File(["archive"], "reviewer.tar.gz", { type: "application/gzip" })] },
  });
  fireEvent.click(await screen.findByRole("button", { name: "Import image" }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
    "https://source/api/image-imports/import-1/apply",
    expect.objectContaining({ method: "POST" }),
  ));
});

it("removes from the route host instead of the active daemon", async () => {
  const source = await addDaemon({ label: "Source", baseURL: "https://source", token: "source-token" });
  setActiveDaemon({ id: "active", label: "Active", baseURL: "https://active", token: "active-token" });
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path === "https://source/api/images" && init?.method === "GET") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [
        { name: "reviewer", tag: "v3", bare: false, exportable: true },
      ] } }), { status: 200 }));
    }
    if (path === "https://source/api/images/reviewer%3Av3" && init?.method === "DELETE") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { removed: "reviewer:v3" } }), { status: 200 }));
    }
    return Promise.reject(new Error(`unexpected request ${init?.method} ${path}`));
  });
  vi.stubGlobal("fetch", fetchMock);

  render(<MemoryRouter><BuiltImages hostId={source.id} /></MemoryRouter>);

  fireEvent.click(await screen.findByRole("button", { name: "Remove reviewer:v3" }));
  fireEvent.click(screen.getByRole("button", { name: "Remove image" }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
    "https://source/api/images/reviewer%3Av3",
    expect.objectContaining({ method: "DELETE" }),
  ));
});

it("waits for the current destination generation before opening a transfer", async () => {
  const source = await addDaemon({ label: "Source", baseURL: "https://source", token: "source-token" });
  const target = await addDaemon({ label: "Target", baseURL: "https://target", token: "target-token" });
  daemonContext.daemons = [{ ...source, state: "ready" }, { ...target, state: "ready" }];
  const resolvedSource = await daemonsApi.resolveDaemon(source.id);
  const resolvedTarget = await daemonsApi.resolveDaemon(target.id);
  let finishTarget: (target: Daemon | null) => void = () => undefined;
  const pendingTarget = new Promise<Daemon | null>((resolve) => { finishTarget = resolve; });
  const resolveDaemon = vi.spyOn(daemonsApi, "resolveDaemon");
  resolveDaemon.mockImplementation((id) => id === target.id ? pendingTarget : Promise.resolve(id === source.id ? resolvedSource : null));
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true, result: { images: [
    { name: "built", tag: "v1", bare: false, exportable: true },
  ] } }), { status: 200 })));

  render(<MemoryRouter><BuiltImages hostId={source.id} /></MemoryRouter>);

  const action = await screen.findByRole("button", { name: "Upload to servers built:v1" });
  expect(action).toBeDisabled();
  finishTarget(resolvedTarget);
  await waitFor(() => expect(action).toBeEnabled());
});

it("does not invoke an upload action bound to the previous route host", async () => {
  const sourceA = await addDaemon({ label: "Source A", baseURL: "https://source-a", token: "source-a-token" });
  const sourceB = await addDaemon({ label: "Source B", baseURL: "https://source-b", token: "source-b-token" });
  daemonContext.daemons = [
    { ...sourceA, state: "ready" },
    { ...sourceB, state: "ready" },
    { id: "target", label: "Target", baseURL: "https://target", state: "ready" },
  ];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL) => {
    if (String(input) === "https://source-a/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [
        { name: "built", tag: "v1", bare: false, exportable: true },
      ] } }), { status: 200 }));
    }
    if (String(input) === "https://source-b/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [] } }), { status: 200 }));
    }
    return Promise.reject(new Error(`unexpected request ${String(input)}`));
  }));

  const RouteTransition = () => {
    const [hostId, setHostId] = useState(sourceA.id);
    useLayoutEffect(() => {
      if (hostId === sourceB.id) {
        const staleAction = screen.queryByRole("button", { name: "Upload to servers built:v1" });
        if (staleAction) fireEvent.click(staleAction);
      }
    }, [hostId]);
    return <>
      <button onClick={() => setHostId(sourceB.id)}>Switch route</button>
      <BuiltImages hostId={hostId} />
    </>;
  };

  render(<MemoryRouter><RouteTransition /></MemoryRouter>);
  await screen.findByRole("button", { name: "Upload to servers built:v1" });

  fireEvent.click(screen.getByRole("button", { name: "Switch route" }));

  expect(screen.queryByRole("dialog", { name: "Transfer image built:v1" })).toBeNull();
});

it("exports runnable images and distinguishes original builds from imports", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true, status: 200,
    text: async () => JSON.stringify({ ok: true, result: { images: [
      { name: "built", tag: "v1", bare: false, exportable: true, source_cwd: "/srv/images/built" },
      { name: "imported", tag: "v1", bare: false, exportable: true, current_agents: ["reviewer"], pending_agents: ["worker"] },
      { name: "missing", tag: "v1", bare: false, exportable: true, source_cwd: "/srv/images/missing", source_available: false },
      { name: "bare", tag: "latest", bare: true, exportable: true, source_cwd: "/srv/images/bare" },
    ] } }),
  } as Response));
  render(
    <MemoryRouter>
      <BuiltImages hostId="" basePath="/servers/local/images" />
    </MemoryRouter>,
  );

  expect(await screen.findByRole("button", { name: "Export built:v1" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Export imported:v1" })).toBeEnabled();
  expect(screen.getByText("/srv/images/built")).toBeInTheDocument();
  expect(screen.getByText("Source CWD unavailable — imported artifact")).toBeInTheDocument();
  expect(screen.getByText("Source CWD unavailable — /srv/images/missing")).toBeInTheDocument();
  expect(screen.getByText("Current: reviewer")).toBeInTheDocument();
  expect(screen.getByText("Pending: worker")).toBeInTheDocument();
  expect(screen.getAllByTitle(/original sources are not included/i)).toHaveLength(3);
  expect(screen.getByLabelText("Import image archive")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "built:v1" }))
    .toHaveAttribute("href", "/servers/local/images/built/v1");
  expect(screen.queryByRole("button", { name: "Upload to servers bare:latest" })).toBeNull();
});

it("downloads the runnable bundle and confirms the saved portable filename", async () => {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const path = String(input);
    if (path === "/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [
        { name: "reviewer", tag: "v3", bare: false, exportable: true },
      ] } }), { status: 200 }));
    }
    if (path === "/api/images/reviewer%3Av3/export") {
      return Promise.resolve({ ok: true, status: 200, blob: () => Promise.resolve(new Blob(["archive"])) } as Response);
    }
    return Promise.reject(new Error(`unexpected request ${path}`));
  });
  vi.stubGlobal("fetch", fetchMock);
  vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:archive");
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
  const success = vi.spyOn(toast, "success");
  let downloaded = "";
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) { downloaded = this.download; });
  render(<MemoryRouter><BuiltImages hostId="" /></MemoryRouter>);

  fireEvent.click(await screen.findByRole("button", { name: "Export reviewer:v3" }));
  await waitFor(() => expect(downloaded).toBe("reviewer-v3.tariboy-image.tar.gz"));
  expect(success).toHaveBeenCalledWith(
    "image reviewer:v3 saved to file reviewer-v3.tariboy-image.tar.gz",
  );
});

it("lets the operator retag a conflicting image import", async () => {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path === "/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [] } }), { status: 200 }));
    }
    if (path === "/api/image-imports" && init?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { import_id: "import-1", ref: "reviewer:v1", digest: "abc" } }), { status: 200 }));
    }
    if (path === "/api/image-imports/import-1/apply" && init?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: {} }), { status: 200 }));
    }
    return Promise.reject(new Error(`unexpected request ${init?.method} ${path}`));
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<MemoryRouter><BuiltImages hostId="" /></MemoryRouter>);

  const archiveInput = screen.getByLabelText("Import image archive");
  await waitFor(() => expect(archiveInput).toBeEnabled());
  fireEvent.change(archiveInput, {
    target: { files: [new File(["archive"], "reviewer.tar.gz", { type: "application/gzip" })] },
  });
  fireEvent.change(await screen.findByLabelText("Import name"), { target: { value: "reviewer-copy" } });
  fireEvent.change(screen.getByLabelText("Import tag"), { target: { value: "v2" } });
  fireEvent.click(screen.getByRole("button", { name: "Import image" }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
    "/api/image-imports/import-1/apply",
    expect.objectContaining({ body: JSON.stringify({ ref: "reviewer-copy:v2" }) }),
  ));
});
