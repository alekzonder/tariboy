import { it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { RetentionPanel } from "./RetentionPanel";

afterEach(() => vi.restoreAllMocks());

const POLICY = { keep_iterations: 5, keep_days: 7, max_bytes: 1024, archive: true };

type Call = { path: string; method?: string; body: unknown };

// stubFetch answers the two requests this panel makes. `prune` decides what a
// prune POST resolves to, so a test can hold one in flight or reject it with a
// specific server reason.
function stubFetch(
  calls: Call[],
  prune?: (body: unknown) => Promise<Response>,
) {
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    const body = init?.body ? JSON.parse(init.body as string) : undefined;
    if (init?.method) calls.push({ path, method: init.method, body });
    if (init?.method === "POST" && path.endsWith("/prune") && prune) return prune(body);
    const result = path.endsWith("/retention") && init?.method !== "POST" ? POLICY : {};
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
}

const rejects = () => Promise.resolve({
  ok: false, status: 400,
  text: async () => JSON.stringify({ ok: false, error: { code: "invalid", message: "prune is not permitted while the agent runs" } }),
} as Response);

const prunes = (calls: Call[]) => calls.filter((c) => c.method === "POST" && c.path.endsWith("/prune"));

it("labels the section, its limits and its timings, and leaves the metadata read-only", async () => {
  const calls: Call[] = [];
  stubFetch(calls);
  render(<RetentionPanel name="alpha" />);

  expect(await screen.findByText("Retention and cleanup")).toBeInTheDocument();
  expect(screen.getByText("Keep the history you need, then review cleanup safely.")).toBeInTheDocument();
  await waitFor(() => expect(screen.getByLabelText("Keep iterations")).toHaveValue("5"));
  expect(screen.getByLabelText("Keep days")).toHaveValue("7");
  // Keep iterations, keep days, the non-destructive actions and prune: four
  // immediate helpers, and no editable control without one.
  expect(screen.getAllByText("Takes effect immediately.")).toHaveLength(4);
  // Archive and max-bytes are displayed policy metadata, not editable fields.
  const meta = screen.getByText(/max_bytes/);
  expect(meta.textContent).toContain("archive: on");
  expect(meta.textContent).not.toContain("Takes effect");
});

it("bounds the destructive action in its own group and does not fire it from the click", async () => {
  const calls: Call[] = [];
  stubFetch(calls);
  render(<RetentionPanel name="alpha" />);

  const group = await screen.findByRole("region", { name: "Delete retained data" });
  expect(group.className).toContain("border-destructive");
  fireEvent.click(screen.getByText("Prune now"));

  // The click only opens the confirmation; nothing is written yet.
  expect(await screen.findByText("Prune retained data?")).toBeInTheDocument();
  expect(screen.getByText(
    "This permanently removes data selected by the current retention policy. Review the preview first if you need to check what will be removed.",
  )).toBeInTheDocument();
  expect(prunes(calls)).toHaveLength(0);
});

it("cancelling the prune confirmation writes nothing and unmounts the dialog", async () => {
  const calls: Call[] = [];
  stubFetch(calls);
  render(<RetentionPanel name="alpha" />);

  await screen.findByText("Prune now");
  expect(screen.queryByText("Cancel")).not.toBeInTheDocument();
  fireEvent.click(screen.getByText("Prune now"));
  fireEvent.click(await screen.findByText("Cancel"));

  await waitFor(() => expect(screen.queryByText("Prune retained data?")).not.toBeInTheDocument());
  expect(screen.queryByText("Prune retained data")).not.toBeInTheDocument();
  expect(prunes(calls)).toHaveLength(0);
});

it("confirming sends the existing prune with dry-run false, exactly once, and reloads", async () => {
  const calls: Call[] = [];
  stubFetch(calls);
  render(<RetentionPanel name="alpha" />);

  await screen.findByText("Prune now");
  const gets = calls.length;
  fireEvent.click(screen.getByText("Prune now"));
  fireEvent.click(await screen.findByText("Prune retained data"));

  await waitFor(() => expect(prunes(calls)).toHaveLength(1));
  expect(prunes(calls)[0].path).toBe("/api/agents/alpha/prune");
  expect(prunes(calls)[0].body).toEqual({ "dry-run": false });
  // The policy is reloaded on success.
  await waitFor(() => expect(calls.length).toBeGreaterThan(gets + 1));
});

it("blocks a duplicate prune while one is in flight, then allows the next one", async () => {
  const calls: Call[] = [];
  // Held in an object: a plain `let` assigned inside the executor keeps its
  // `null` type through control-flow narrowing.
  const held: { release?: () => void } = {};
  stubFetch(calls, () => new Promise<Response>((resolve) => {
    held.release = () => resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: {} }) } as Response);
  }));
  render(<RetentionPanel name="alpha" />);

  await screen.findByText("Prune now");
  fireEvent.click(screen.getByText("Prune now"));
  fireEvent.click(await screen.findByText("Prune retained data"));
  await waitFor(() => expect(prunes(calls)).toHaveLength(1));

  // While the request is in flight the only entry point is disabled, so a
  // second submission cannot be started at all.
  await waitFor(() => expect(screen.getByText("Prune now")).toBeDisabled());
  expect(screen.queryByText("Prune retained data")).not.toBeInTheDocument();

  held.release?.();
  // The transition back proves the disabled state was the in-flight guard and
  // not a permanent dead end.
  await waitFor(() => expect(screen.getByText("Prune now")).toBeEnabled());
  fireEvent.click(screen.getByText("Prune now"));
  fireEvent.click(await screen.findByText("Prune retained data"));
  await waitFor(() => expect(prunes(calls)).toHaveLength(2));
});

it("reports the server's own reason for a failed prune and leaves a retry path", async () => {
  const calls: Call[] = [];
  let failing = true;
  stubFetch(calls, () => (failing
    ? rejects()
    : Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: {} }) } as Response)));
  render(<RetentionPanel name="alpha" />);

  await screen.findByText("Prune now");
  fireEvent.click(screen.getByText("Prune now"));
  fireEvent.click(await screen.findByText("Prune retained data"));

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("prune is not permitted while the agent runs");

  // Retry: the control is live again and a successful attempt clears the error.
  failing = false;
  await waitFor(() => expect(screen.getByText("Prune now")).toBeEnabled());
  fireEvent.click(screen.getByText("Prune now"));
  fireEvent.click(await screen.findByText("Prune retained data"));
  await waitFor(() => expect(prunes(calls)).toHaveLength(2));
  await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
});

it("keeps the non-destructive preview separate and still a dry run", async () => {
  const calls: Call[] = [];
  stubFetch(calls);
  render(<RetentionPanel name="alpha" />);

  fireEvent.click(await screen.findByText("Preview cleanup"));
  await waitFor(() => expect(prunes(calls)).toHaveLength(1));
  expect(prunes(calls)[0].body).toEqual({ "dry-run": true });
  // No confirmation for a preview: it removes nothing.
  expect(screen.queryByText("Prune retained data?")).not.toBeInTheDocument();
});
