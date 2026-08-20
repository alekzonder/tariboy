import { it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import AgentCreate from "./AgentCreate";
import { RUNTIME_PRESETS_STORAGE_KEY } from "@/lib/runtimePresets";

afterEach(() => vi.restoreAllMocks());
beforeEach(() => localStorage.clear());

const resp = (body: unknown) =>
  Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);

function mockApi(posts: unknown[], createSucceeds = true, deferOtherManifest = false) {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    if (path === "/api/agents" && method === "POST") {
      posts.push(init?.body ? JSON.parse(init.body as string) : undefined);
      if (!createSucceeds) {
        return resp({ ok: false, error: { code: "rejected", message: "rejected" } });
      }
      return resp({ ok: true, result: { name: "gen-1", state: "running" } });
    }
    if (path === "/api/images/img%3A1") {
      return resp({ ok: true, result: {
        schema_version: 1, name: "img", tag: "1", built_at: "2026-08-11T00:00:00Z",
        parents: null, plugins: null, requires_secrets: null,
        harness: { type: "codex", model: "gpt-5", effort: "high", interactive: false },
        env: null, policy: {}, evals: null, layers: null,
      } });
    }
    if (path === "/api/images/other%3A1" && deferOtherManifest) {
      return new Promise<Response>(() => {});
    }
    if (path === "/api/images/v2%3Alatest") {
      return resp({ ok: true, result: {
        schema_version: 2, name: "v2", tag: "latest", built_at: "2026-08-17T00:00:00Z",
        parents: null, plugins: [{ name: "context" }], requires_secrets: null,
        env: null, evals: null, layers: null,
      } });
    }
    if (path.startsWith("/api/images"))
      return resp({ ok: true, result: { images: [
        { name: "img", tag: "1", schema_version: 1 },
        { name: "other", tag: "1", schema_version: 1 },
        { name: "v2", tag: "latest", schema_version: 2 },
      ], count: 3 } });
    if (path.startsWith("/api/groups"))
      return resp({ ok: true, result: { groups: [], count: 0 } });
    if (path.startsWith("/api/fs/list"))
      return resp({ ok: true, result: { path: "", parent: "", entries: [] } });
    return resp({ ok: true, result: {} });
  }));
}

it("gates submit on image and serializes env/plugins in the POST payload", async () => {
  const posts: unknown[] = [];
  mockApi(posts);
  render(<MemoryRouter><AgentCreate /></MemoryRouter>);

  // image required: submit is disabled until an image is chosen.
  const create = screen.getByRole("button", { name: /Create agent/i });
  expect(create).toBeDisabled();

  // Open the image combobox and pick the one image.
  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "img:1" }));
  await waitFor(() => expect(create).toBeEnabled());

  // Reveal the Advanced block (env repeater + plugin chips live there).
  fireEvent.click(screen.getByRole("button", { name: /Advanced/i }));

  fireEvent.click(screen.getByRole("button", { name: /Add env/i }));
  fireEvent.change(screen.getByLabelText("env key 0"), { target: { value: "KEY" } });
  fireEvent.change(screen.getByLabelText("env value 0"), { target: { value: "val" } });

  fireEvent.change(screen.getByPlaceholderText("plugin name"), { target: { value: "myplugin" } });
  fireEvent.click(screen.getByRole("button", { name: /Add plugin/i }));

  fireEvent.click(create);

  await waitFor(() => expect(posts.length).toBe(1));
  expect(posts[0]).toMatchObject({
    image: "img:1",
    env: "KEY=val",
    plugins: "myplugin",
    loop: true,
    interactive: false,
  });
});

it("omits empty optional fields from the payload", async () => {
  const posts: unknown[] = [];
  mockApi(posts);
  render(<MemoryRouter><AgentCreate /></MemoryRouter>);

  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "img:1" }));
  await waitFor(() => expect(screen.getByRole("button", { name: /Create agent/i })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: /Create agent/i }));

  await waitFor(() => expect(posts.length).toBe(1));
  const body = posts[0] as Record<string, unknown>;
  expect(body.image).toBe("img:1");
  // Untouched optional fields serialize to undefined (dropped by JSON.stringify).
  expect("env" in body).toBe(false);
  expect("plugins" in body).toBe(false);
  expect("cwd" in body).toBe(false);
  expect("name" in body).toBe(false);
});

it("does not offer or submit plugin overrides for schema v2 images", async () => {
  const posts: unknown[] = [];
  mockApi(posts);
  render(<MemoryRouter><AgentCreate /></MemoryRouter>);

  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "v2:latest" }));
  await waitFor(() => expect(screen.getByRole("button", { name: /Create agent/i })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: /Advanced/i }));

  expect(screen.queryByPlaceholderText("plugin name")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /Create agent/i }));
  await waitFor(() => expect(posts).toHaveLength(1));
  expect("plugins" in (posts[0] as Record<string, unknown>)).toBe(false);
});

it("preselects the image supplied by the shared Run Agent flow", async () => {
  const posts: unknown[] = [];
  mockApi(posts);
  render(
    <MemoryRouter initialEntries={["/agents/new?image=img%3A1"]}>
      <AgentCreate />
    </MemoryRouter>,
  );

  expect(await screen.findByLabelText("image")).toHaveValue("img:1");
  await waitFor(() => expect(screen.getByRole("button", { name: /Create agent/i })).toBeEnabled());
});

it("remembers successful custom runtime values from the page form", async () => {
  const posts: unknown[] = [];
  mockApi(posts, true, true);
  render(<MemoryRouter><AgentCreate /></MemoryRouter>);

  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "img:1" }));
  await screen.findByDisplayValue("gpt-5");
  fireEvent.change(screen.getByLabelText("model"), { target: { value: "private-model" } });
  fireEvent.change(screen.getByLabelText("effort"), { target: { value: "ultra-custom" } });
  fireEvent.click(screen.getByRole("button", { name: /Create agent/i }));

  await waitFor(() => expect(posts).toHaveLength(1));
  expect(posts[0]).toMatchObject({
    image: "img:1", model: "private-model", effort: "ultra-custom",
  });
  expect(JSON.parse(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY) ?? "{}")).toEqual({
    codex: { models: ["private-model"], efforts: ["ultra-custom"] },
  });
});

it("does not remember custom runtime values when creation from the page fails", async () => {
  const posts: unknown[] = [];
  mockApi(posts, false);
  render(<MemoryRouter><AgentCreate /></MemoryRouter>);

  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "img:1" }));
  await screen.findByDisplayValue("gpt-5");
  fireEvent.change(screen.getByLabelText("model"), { target: { value: "private-model" } });
  fireEvent.click(screen.getByRole("button", { name: /Create agent/i }));

  await screen.findByText("rejected");
  expect(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY)).toBeNull();
});

it("waits for a newly selected image manifest before it enables creation", async () => {
  const posts: unknown[] = [];
  mockApi(posts);
  render(<MemoryRouter initialEntries={["/agents/new?image=img%3A1"]}><AgentCreate /></MemoryRouter>);

  await screen.findByDisplayValue("gpt-5");
  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "other:1" }));

  expect(screen.getByRole("button", { name: /Create agent/i })).toBeDisabled();
});
