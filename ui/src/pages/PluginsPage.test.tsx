import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import PluginsPage from "./PluginsPage";

afterEach(() => vi.restoreAllMocks());

it("lists plugins and fetches logs on demand", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => {
    let body: unknown = { ok: true, result: {} };
    if (path === "/api/plugins") body = { ok: true, result: { plugins: [{ name: "status", version: "1.0", types: ["sink"], enabled: true, state: "running" }], count: 1 } };
    else if (path.endsWith("/logs")) body = { ok: true, result: { lines: ["line-one"], count: 1 } };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);
  }));
  render(<PluginsPage />);

  await waitFor(() => expect(screen.getByText("status")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Logs"));
  await waitFor(() => expect(screen.getByText("line-one")).toBeInTheDocument());
});

it("restarts a plugin via the Restart button", async () => {
  const fetchMock = vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    let body: unknown = { ok: true, result: {} };
    if (path === "/api/plugins" && (!init || init.method === undefined || init.method === "GET"))
      body = { ok: true, result: { plugins: [{ name: "messenger", version: "0.1.0", types: ["channel-sink"], enabled: true, state: "running" }], count: 1 } };
    else if (path === "/api/plugins/messenger/restart") body = { ok: true, result: { restarted: "messenger" } };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<PluginsPage />);

  await waitFor(() => expect(screen.getByText("messenger")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Restart"));
  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith("/api/plugins/messenger/restart", expect.objectContaining({ method: "POST" })),
  );
});

it("does not expose plugin-specific route management", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string) => {
    let result: unknown = {};
    if (url === "/api/plugins") result = { plugins: [
      { name: "messenger-provider", version: "0.1.0", types: ["channel-source", "channel-sink"], enabled: true, state: "running" },
      { name: "status", version: "1.0.0", types: ["tool"], enabled: true, state: "running" },
    ], count: 2 };
    else if (url.includes("/routes")) result = { routes: {}, default_route: "", has_token: true };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
  render(<PluginsPage />);
  await waitFor(() => expect(screen.getByText("messenger-provider")).toBeInTheDocument());
  expect(screen.queryByRole("button", { name: "Routes" })).not.toBeInTheDocument();
  expect(screen.queryByText(/chats/i)).not.toBeInTheDocument();
});
