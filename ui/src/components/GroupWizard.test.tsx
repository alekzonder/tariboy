import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { GroupWizard } from "./GroupWizard";

afterEach(() => vi.restoreAllMocks());

const resp = (body: unknown, ok = true, status = 200) =>
  Promise.resolve({ ok, status, text: async () => JSON.stringify(body) } as Response);

interface Call { path: string; method: string; body: unknown }

// mockApi records every /api/groups and /api/agents POST into `calls` (in
// order). `failAgentImages` is a set of image refs whose agent POST fails, so a
// test can drive the partial-failure path.
function mockApi(calls: Call[], failAgentImages = new Set<string>(), requests?: string[]) {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    requests?.push(path);
    const method = init?.method ?? "GET";
    const body = init?.body ? JSON.parse(init.body as string) : undefined;
    if (path === "/api/groups" && method === "POST") {
      calls.push({ path, method, body });
      return resp({ ok: true, result: { name: body.name, lead: body.lead ?? "" } });
    }
    if (path === "/api/agents" && method === "POST") {
      calls.push({ path, method, body });
      if (failAgentImages.has(body.image as string))
        return resp({ ok: false, error: { code: "bad_cwd", message: "no such dir" } }, false, 400);
      return resp({ ok: true, result: { name: (body.name as string) || "gen", state: "running" } });
    }
    if (path.startsWith("/api/images"))
      return resp({
        ok: true,
        result: { images: [
          { name: "img", tag: "1", schema_version: 1 },
          { name: "img", tag: "2", schema_version: 2 },
        ], count: 2 },
      });
    if (path.startsWith("/api/fs/list"))
      return resp({ ok: true, result: { path: "", parent: "", entries: [] } });
    return resp({ ok: true, result: {} });
  }));
}

it("offers custom teams without requesting deleted built-in templates", async () => {
  const calls: Call[] = [];
  const requests: string[] = [];
  mockApi(calls, new Set(), requests);
  render(<GroupWizard />);

  expect(screen.queryByLabelText("team template")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Add agent/i })).toBeInTheDocument();
  await waitFor(() => expect(requests.some((path) => path.startsWith("/api/images"))).toBe(true));
  expect(requests.some((path) => path.startsWith("/api/team-templates"))).toBe(false);
  expect(calls).toHaveLength(0);
});

// Pick an image in the row at index `i` via its per-row combobox.
async function pickImage(i: number, ref: string) {
  fireEvent.focus(screen.getByLabelText(`image ${i}`));
  fireEvent.click(await screen.findByRole("option", { name: ref }));
}

it("orchestrates N+1 calls in order: POST /api/groups then one POST /api/agents per row", async () => {
  const calls: Call[] = [];
  mockApi(calls);
  render(<GroupWizard />);

  // Section 1: group name.
  fireEvent.change(screen.getByLabelText("group name *"), { target: { value: "squad" } });

  // Row 0 (default): image + name; row 0 is the default leader marker.
  await pickImage(0, "img:1");
  fireEvent.change(screen.getByLabelText("name"), { target: { value: "boss" } });

  // Add a second agent row and fill it.
  fireEvent.click(screen.getByRole("button", { name: /Add agent/i }));
  await pickImage(1, "img:2");

  // Submit is gated until both rows have an image and the group has a name.
  const create = screen.getByRole("button", { name: /^Create group$/i });
  expect(create).toBeEnabled();
  fireEvent.click(create);

  // 1 group + 2 agents = 3 calls, in order.
  await waitFor(() => expect(calls.length).toBe(3));
  expect(calls[0].path).toBe("/api/groups");
  expect(calls[0].body).toMatchObject({ name: "squad", lead: "boss" });
  expect(calls[1].path).toBe("/api/agents");
  expect(calls[1].body).toMatchObject({ image: "img:1", name: "boss", group: "squad", loop: true });
  expect(calls[2].path).toBe("/api/agents");
  expect(calls[2].body).toMatchObject({ image: "img:2", group: "squad" });
});

it("blocks submit until the leader row has an explicit name — never a leaderless group", async () => {
  const calls: Call[] = [];
  mockApi(calls);
  render(<GroupWizard />);

  // Group name + image on the (default) leader row, but leave its name blank.
  fireEvent.change(screen.getByLabelText("group name *"), { target: { value: "squad" } });
  await pickImage(0, "img:1");

  // Submit is gated: a blank leader name would POST /api/groups with lead
  // undefined (leaderless), so the button stays disabled and explains why.
  const create = screen.getByRole("button", { name: /^Create group$/i });
  expect(create).toBeDisabled();
  expect(screen.getByText(/leader row needs an explicit name/i)).toBeTruthy();

  // Clicking the disabled button issues no calls — no leaderless group is born.
  fireEvent.click(create);
  await new Promise((r) => setTimeout(r, 0));
  expect(calls.length).toBe(0);

  // Naming the leader row unblocks submit and the group is created with a lead.
  fireEvent.change(screen.getByLabelText("name"), { target: { value: "boss" } });
  expect(create).toBeEnabled();
  fireEvent.click(create);
  await waitFor(() => expect(calls.length).toBe(2));
  expect(calls[0].path).toBe("/api/groups");
  expect(calls[0].body).toMatchObject({ name: "squad", lead: "boss" });
});

it("base cwd seeds every row; a per-row override wins for that row only", async () => {
  const calls: Call[] = [];
  mockApi(calls);
  render(<GroupWizard />);

  fireEvent.change(screen.getByLabelText("group name *"), { target: { value: "squad" } });
  fireEvent.change(screen.getByLabelText("base cwd"), { target: { value: "/home/base" } });
  await pickImage(0, "img:1");
  // Leader row needs an explicit name for submit to be enabled.
  fireEvent.change(screen.getByLabelText("name"), { target: { value: "boss" } });

  fireEvent.click(screen.getByRole("button", { name: /Add agent/i }));
  await pickImage(1, "img:2");
  // Toggle row 1's override switch (second one in DOM order), then type a path
  // into the revealed row-1 cwd autocomplete.
  const overrideSwitches = screen.getAllByLabelText("override cwd");
  fireEvent.click(overrideSwitches[1]);
  fireEvent.change(screen.getByLabelText("cwd 1"), { target: { value: "/home/special" } });

  fireEvent.click(screen.getByRole("button", { name: /^Create group$/i }));
  await waitFor(() => expect(calls.length).toBe(3));
  expect(calls[1].body).toMatchObject({ cwd: "/home/base" });   // row 0 → base
  expect(calls[2].body).toMatchObject({ cwd: "/home/special" }); // row 1 → override
});

it("keeps plugin overrides exclusive to schema v1 image rows", async () => {
  const calls: Call[] = [];
  mockApi(calls);
  render(<GroupWizard />);

  fireEvent.change(screen.getByLabelText("group name *"), { target: { value: "squad" } });
  await pickImage(0, "img:1");
  fireEvent.change(screen.getByLabelText("name"), { target: { value: "boss" } });
  fireEvent.click(screen.getByRole("button", { name: /Advanced/i }));
  fireEvent.change(screen.getByPlaceholderText("plugin name"), { target: { value: "legacy" } });
  fireEvent.click(screen.getByRole("button", { name: /Add plugin/i }));

  await pickImage(0, "img:2");
  expect(screen.queryByPlaceholderText("plugin name")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /^Create group$/i }));

  await waitFor(() => expect(calls).toHaveLength(2));
  expect("plugins" in (calls[1].body as Record<string, unknown>)).toBe(false);
});

it("renders partial-failure state and retries only the failed row", async () => {
  const calls: Call[] = [];
  // Any agent created from img:2 fails.
  mockApi(calls, new Set(["img:2"]));
  render(<GroupWizard />);

  fireEvent.change(screen.getByLabelText("group name *"), { target: { value: "squad" } });
  await pickImage(0, "img:1");
  // Leader row needs an explicit name for submit to be enabled.
  fireEvent.change(screen.getByLabelText("name"), { target: { value: "boss" } });
  fireEvent.click(screen.getByRole("button", { name: /Add agent/i }));
  await pickImage(1, "img:2");

  fireEvent.click(screen.getByRole("button", { name: /^Create group$/i }));

  // 3 calls; row 1 failed with its reason shown, row 0 created.
  await waitFor(() => expect(calls.length).toBe(3));
  await screen.findByText(/failed: no such dir/i);
  // Row 0 (img:1, named "boss") shows its created badge.
  expect(screen.getByText(/created boss/i)).toBeTruthy();

  // Retry re-issues ONLY the failed agent — no new group call, no re-create of
  // the succeeded row. (Still failing, so it's one more /api/agents call.)
  fireEvent.click(await screen.findByRole("button", { name: /Retry failed/i }));
  await waitFor(() => expect(calls.length).toBe(4));
  expect(calls[3].path).toBe("/api/agents");
  expect(calls[3].body).toMatchObject({ image: "img:2" });
  // No second group create.
  expect(calls.filter((c) => c.path === "/api/groups").length).toBe(1);
});
