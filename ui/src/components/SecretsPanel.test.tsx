import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SecretsPanel } from "./SecretsPanel";

afterEach(() => vi.restoreAllMocks());

it("lists key names only and never renders a value; stores via POST", async () => {
  const posts: Array<{ path: string; body: Record<string, unknown> }> = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    if (init?.method === "POST") posts.push({ path, body: JSON.parse(init.body as string) });
    const body = init?.method === undefined || init.method === "GET"
      ? { ok: true, result: { keys: ["API_KEY"], count: 1 } }
      : { ok: true, result: {} };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify(body) } as Response);
  }));
  render(<SecretsPanel name="alpha" />);

  await waitFor(() => expect(screen.getByText("API_KEY")).toBeInTheDocument());
  fireEvent.change(screen.getByPlaceholderText("KEY"), { target: { value: "TOKEN" } });
  fireEvent.change(screen.getByPlaceholderText("value (never shown)"), { target: { value: "s3cr3t" } });
  fireEvent.click(screen.getByText("Store secret"));
  await waitFor(() => expect(posts.some((p) => p.path === "/api/agents/alpha/secrets" && p.body.key === "TOKEN")).toBe(true));
  // the value must never appear in the DOM as text
  expect(screen.queryByText("s3cr3t")).toBeNull();
});

// The empty state lives here rather than in AgentSettings.test.tsx because the
// string belongs to this component and this is the only file that can choose
// the empty and non-empty key lists independently.
it("invites a first secret instead of only reporting that there are none", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation(() =>
    Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: { keys: [], count: 0 } }) } as Response)));
  render(<SecretsPanel name="alpha" />);

  expect(await screen.findByText("Add a secret key and value to make it available to the agent.")).toBeInTheDocument();
  expect(screen.queryByText("No secrets.")).not.toBeInTheDocument();
  // Both write controls are programmatically labelled and carry the one timing
  // string for storing or removing a secret.
  expect(screen.getByLabelText("Secret key")).toBeInTheDocument();
  expect(screen.getByLabelText("Secret value")).toBeInTheDocument();
  expect(screen.getByText("Takes effect on the next iteration.")).toBeInTheDocument();
});
