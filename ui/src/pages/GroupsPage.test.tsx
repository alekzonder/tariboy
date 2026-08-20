import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import GroupsPage from "./GroupsPage";
import { MemoryRouter } from "react-router-dom";

afterEach(() => vi.restoreAllMocks());

it("lists groups and creates a new one", async () => {
  const posts: Array<{ path: string; body: unknown }> = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    if (init?.method === "POST") posts.push({ path, body: init?.body ? JSON.parse(init.body as string) : undefined });
    const body = path === "/api/groups" && (!init || init.method === "GET" || init.method === undefined)
      ? { ok: true, result: { groups: [{ name: "team", lead: "boss", members: 2 }], count: 1 } }
      : { ok: true, result: {} };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);
  }));
  render(<MemoryRouter><GroupsPage /></MemoryRouter>);

  await waitFor(() => expect(screen.getByText("team")).toBeInTheDocument());
  fireEvent.change(screen.getByPlaceholderText("group name"), { target: { value: "newg" } });
  fireEvent.click(screen.getByText("Create"));
  await waitFor(() => expect(posts.some((p) => p.path === "/api/groups" && (p.body as { name?: string })?.name === "newg")).toBe(true));
});

it("edits members and exposes compose and portable archive actions", async () => {
  const calls: Array<{ path: string; method: string; body?: unknown }> = [];
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    calls.push({ path, method, body: typeof init?.body === "string" ? JSON.parse(init.body) : init?.body });
    let result: unknown = {};
    if (path === "/api/groups") result = { groups: [{ name: "team", lead: "boss", members: 2 }], count: 1 };
    if (path === "/api/groups/team") result = { name: "team", lead: "boss", members: ["boss", "worker"], shared_dir: "/shared" };
    if (path === "/api/groups/team/compose") result = { name: "team", yaml: "version: 1\n" };
    if (path === "/api/groups/team/export") return Promise.resolve({ ok: true, status: 200, blob: async () => new Blob(["archive"]) } as Response);
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
  render(<MemoryRouter><GroupsPage /></MemoryRouter>);

  fireEvent.click(await screen.findByText("team"));
  fireEvent.click(await screen.findByRole("button", { name: "Remove worker" }));
  await waitFor(() => expect(calls.some((call) => call.method === "DELETE" && call.path === "/api/groups/team/members/worker")).toBe(true));

  fireEvent.click(screen.getByRole("button", { name: "Copy compose YAML" }));
  await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith("version: 1\n"));
  expect(screen.getByRole("button", { name: "Export team archive" })).toBeInTheDocument();
  expect(screen.getByLabelText("Import compose YAML")).toBeInTheDocument();
  expect(screen.getByLabelText("Import team archive")).toBeInTheDocument();
});

it("previews a team archive and waits for explicit confirmation before apply", async () => {
  const paths: string[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    paths.push(`${init?.method ?? "GET"} ${path}`);
    if (path === "/api/team-imports") return Promise.resolve({ ok: true, status: 200, json: async () => ({ result: { import_id: "imp-1", team: "portable", yaml: "version: 1", images: [{ ref: "img:v1" }] } }) } as Response);
    const result = path === "/api/groups" ? { groups: [] } : path === "/api/team-imports/imp-1" ? { import_id: "imp-1", team: "portable", status: "complete", steps: [], updated_at: "now" } : { complete: true };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
  render(<MemoryRouter><GroupsPage /></MemoryRouter>);
  fireEvent.change(await screen.findByLabelText("Import team archive"), { target: { files: [new File(["archive"], "team.tar.gz", { type: "application/gzip" })] } });

  expect(await screen.findByText("portable")).toBeInTheDocument();
  expect(screen.getByLabelText("Imported team image ref 0")).toHaveValue("img:v1");
  expect(paths).not.toContain("POST /api/team-imports/imp-1/apply");
  fireEvent.click(screen.getByRole("button", { name: "Confirm and import team" }));
  await waitFor(() => expect(paths).toContain("POST /api/team-imports/imp-1/apply"));
});

it("previews pasted YAML and requires an explicit existing-team resolution", async () => {
  const calls: Array<{ path: string; body?: Record<string, unknown> }> = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    const body = typeof init?.body === "string" ? JSON.parse(init.body) as Record<string, unknown> : undefined;
    calls.push({ path, body });
    const result = path === "/api/groups" ? { groups: [] }
      : path === "/api/team-imports/preview-yaml" ? { import_id: "yaml-1", team: "team", yaml: "version: 1\ngroups:\n  team: {}\n", images: [], agents: [], groups: [{ name: "team", action: "choose", conflict: true, message: "destination team exists" }] }
      : path === "/api/team-imports/yaml-1" ? { import_id: "yaml-1", team: "team", status: "complete", steps: [], updated_at: "now" }
      : { complete: true };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
  render(<MemoryRouter><GroupsPage /></MemoryRouter>);
  fireEvent.change(screen.getByLabelText("Import compose YAML"), { target: { value: "version: 1\ngroups:\n  team: {}\n" } });
  fireEvent.click(screen.getByRole("button", { name: "Preview YAML" }));
  expect(await screen.findByText(/destination team exists/)).toBeInTheDocument();
  expect(calls.some((call) => call.path === "/api/team-imports/yaml-1/apply")).toBe(false);
  fireEvent.click(screen.getByLabelText("Update existing team"));
  fireEvent.click(screen.getByRole("button", { name: "Confirm and import team" }));
  await waitFor(() => expect(calls.find((call) => call.path === "/api/team-imports/yaml-1/apply")?.body?.update_existing).toBe(true));
});

it("opens the team named by the workspace management link", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: path === "/api/groups/team" ? { name: "team", lead: "boss", members: ["boss"], shared_dir: "/shared" } : { groups: [{ name: "team", lead: "boss", members: 1 }] } }) } as Response)));
  render(<MemoryRouter initialEntries={["/settings/advanced/groups?team=team"]}><GroupsPage /></MemoryRouter>);
  expect(await screen.findByLabelText("team name")).toHaveValue("team");
});
