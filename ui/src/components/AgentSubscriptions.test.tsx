import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentSubscriptions, validChannel } from "./AgentSubscriptions";

afterEach(() => vi.restoreAllMocks());

interface Post { method: string; url: string; body?: string }

function stub(posts: Array<Post>) {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    if (method !== "GET") posts.push({ method, url, body: init?.body as string | undefined });
    let result: unknown = {};
    if (url.includes("/subscriptions")) result = { channels: [
      { name: "agent:worker:inbox", kind: "agent_inbox", protected: true },
      { name: "group:dev:inbox", kind: "group_inbox", protected: true },
      { name: "chat:messenger:x", kind: "chat", protected: false },
    ], count: 3 };
    // chat:messenger:idle is a bound-but-idle chat W1 (dbu.1) now surfaces here; it
    // is NOT in the subscriptions list, so it must show up as a Subscribe option.
    else if (url === "/api/channels") result = { channels: [
      { name: "chat:messenger:x", kind: "chat" }, { name: "chat:room", kind: "chat" },
      { name: "chat:messenger:idle", kind: "chat" },
    ], count: 3 };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
}

it("renders the left column at 40% width (w-2/5)", async () => {
  stub([]);
  render(<AgentSubscriptions name="worker" />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  // The SUBSCRIPTIONS label sits directly inside the column root, which carries
  // the 40%-width class plus the min-width floor.
  const column = screen.getByText("SUBSCRIPTIONS").parentElement!;
  expect(column).toHaveClass("w-2/5");
  expect(column).toHaveClass("min-w-[12rem]");
});

it("marks protected rows and only offers Unsubscribe on ad-hoc ones", async () => {
  stub([]);
  render(<AgentSubscriptions name="worker" />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  // exactly one Unsubscribe button (the ad-hoc chat), none for protected rows.
  expect(screen.getAllByRole("button", { name: "Unsubscribe" })).toHaveLength(1);
});

it("clicking a row calls onSelect and the selected row is highlighted", async () => {
  stub([]);
  const onSelect = vi.fn();
  const { rerender } = render(<AgentSubscriptions name="worker" onSelect={onSelect} />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());

  // Clicking the row body selects the channel (the row, not the Unsubscribe button).
  fireEvent.click(screen.getByText("chat:messenger:x"));
  expect(onSelect).toHaveBeenCalledWith("chat:messenger:x");

  // With that channel selected, the row wrapper (the non-interactive container
  // holding the name button) carries the highlight class.
  rerender(<AgentSubscriptions name="worker" selected="chat:messenger:x" onSelect={onSelect} />);
  const row = screen.getByText("chat:messenger:x").closest("div")!;
  expect(row).toHaveClass("bg-accent");
});

it("activates a subscription row from the keyboard (native button)", async () => {
  stub([]);
  const onSelect = vi.fn();
  render(<AgentSubscriptions name="worker" onSelect={onSelect} />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  // The selectable region is a native <button>, so it is focusable and Enter/
  // Space activate it with no custom onKeyDown — F1 from the adm.3 review.
  const row = screen.getByRole("button", { name: "chat:messenger:x" });
  row.focus();
  expect(row).toHaveFocus();
  await userEvent.keyboard("{Enter}");
  expect(onSelect).toHaveBeenCalledWith("chat:messenger:x");
  await userEvent.keyboard(" ");
  expect(onSelect).toHaveBeenCalledTimes(2);
});

it("Unsubscribe is a sibling of the name button and does not select the row", async () => {
  stub([]);
  const onSelect = vi.fn();
  render(<AgentSubscriptions name="worker" onSelect={onSelect} />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: "Unsubscribe" }));
  expect(onSelect).not.toHaveBeenCalled();
});

it("unsubscribes an ad-hoc channel via DELETE with a channel query param", async () => {
  const posts: Array<{ method: string; url: string }> = [];
  stub(posts);
  render(<AgentSubscriptions name="worker" />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: "Unsubscribe" }));
  await waitFor(() => expect(posts.some((p) =>
    p.method === "DELETE" && p.url.includes("/api/agents/worker/subscriptions") && p.url.includes("channel=chat%3Amessenger%3Ax"))).toBe(true));
});

it("offers a bound-but-idle chat as a Subscribe option", async () => {
  stub([]);
  render(<AgentSubscriptions name="worker" />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  // Focus the picker to open the dropdown; the bound idle chat appears as an
  // option (already-subscribed chat:messenger:x is filtered out).
  const input = screen.getByLabelText("channel");
  await userEvent.click(input);
  expect(await screen.findByRole("option", { name: /chat:messenger:idle/ })).toBeInTheDocument();
  expect(screen.queryByRole("option", { name: /chat:messenger:x$/ })).not.toBeInTheDocument();
});

it("subscribes to a free-text channel not in the list", async () => {
  const posts: Array<Post> = [];
  stub(posts);
  render(<AgentSubscriptions name="worker" />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  const input = screen.getByLabelText("channel");
  await userEvent.click(input);
  await userEvent.type(input, "chat:custom:room");
  fireEvent.click(screen.getByRole("button", { name: "Subscribe" }));
  await waitFor(() => {
    const p = posts.find((p) => p.method === "POST" && p.url.includes("/api/agents/worker/subscriptions"));
    expect(p).toBeTruthy();
    expect(JSON.parse(p!.body!)).toEqual({ channel: "chat:custom:room" });
  });
});

it("subscribes to a free-text channel via the Enter key", async () => {
  const posts: Array<Post> = [];
  stub(posts);
  render(<AgentSubscriptions name="worker" />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  const input = screen.getByLabelText("channel");
  await userEvent.click(input);
  await userEvent.type(input, "chat:custom:room");
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => {
    const p = posts.find((p) => p.method === "POST" && p.url.includes("/api/agents/worker/subscriptions"));
    expect(p).toBeTruthy();
    expect(JSON.parse(p!.body!)).toEqual({ channel: "chat:custom:room" });
  });
});

it("rejects an invalid free-text channel inline and never POSTs", async () => {
  const posts: Array<Post> = [];
  stub(posts);
  render(<AgentSubscriptions name="worker" />);
  await waitFor(() => expect(screen.getByText("chat:messenger:x")).toBeInTheDocument());
  const input = screen.getByLabelText("channel");
  await userEvent.click(input);
  await userEvent.type(input, "Not A Channel");
  // Inline error shown and the Subscribe button is disabled.
  expect(screen.getByRole("alert")).toBeInTheDocument();
  const btn = screen.getByRole("button", { name: "Subscribe" });
  expect(btn).toBeDisabled();
  fireEvent.click(btn);
  // No POST fired for the invalid input.
  expect(posts.some((p) => p.method === "POST")).toBe(false);
});

it("validChannel mirrors the daemon bus.ValidChannel rules", () => {
  expect(validChannel("chat:messenger:x")).toBe(true);
  expect(validChannel("group:dev:inbox")).toBe(true);
  expect(validChannel("agent:w-1:inbox")).toBe(true);
  expect(validChannel("")).toBe(false);
  expect(validChannel("noprefix")).toBe(false);       // no ':'
  expect(validChannel("bogus:x")).toBe(false);          // unknown prefix
  expect(validChannel("chat:Upper")).toBe(false);       // uppercase segment
  expect(validChannel("chat:_lead")).toBe(false);       // segment starts with '_'
  expect(validChannel("chat:a b")).toBe(false);         // space in segment
});
