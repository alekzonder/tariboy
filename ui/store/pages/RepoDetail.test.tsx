import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import RepoDetail from "./RepoDetail";

vi.mock("../lib/storeApi", () => ({ getTags: vi.fn(), getManifest: vi.fn() }));
import { getTags, getManifest } from "../lib/storeApi";

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/repo/:name" element={<RepoDetail />} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => vi.clearAllMocks());

describe("RepoDetail", () => {
  it("shows version history, manifest details, and pull instructions", async () => {
    (getTags as ReturnType<typeof vi.fn>).mockResolvedValue({
      name: "demo",
      tags: [
        { tag: "latest", digest: "deadbeef", built_at: "2026-07-06T00:00:00Z", pushed_at: "2026-07-06T01:00:00Z" },
      ],
    });
    (getManifest as ReturnType<typeof vi.fn>).mockResolvedValue({
      schema_version: 1, name: "demo", tag: "latest", digest: "deadbeef", built_at: "2026-07-06T00:00:00Z",
      parents: [], plugins: [{ name: "status", version: ">=1.0" }], requires_secrets: ["OPENAI_API_KEY"],
      harness: { type: "claude", model: "opus", interactive: false }, env: {}, policy: {},
      evals: [{ name: "smoke", type: "prompt", prompt: "hi" }], layers: [],
    });
    renderAt("/repo/demo");
    await waitFor(() => expect(screen.getByText("deadbeef")).toBeInTheDocument());
    expect(screen.getByText(/status/)).toBeInTheDocument();
    expect(screen.getByText(/OPENAI_API_KEY/)).toBeInTheDocument();
    expect(screen.getByText(/claude/)).toBeInTheDocument();
    // Pull instruction present.
    expect(screen.getByText(/tariboy pull demo:latest --registry/)).toBeInTheDocument();
  });

  it("renders a tag error without crashing", async () => {
    (getTags as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("boom"));
    renderAt("/repo/demo");
    await waitFor(() => expect(screen.getByText(/boom/)).toBeInTheDocument());
  });

  it("keeps the latest-selected tag's manifest when an older request resolves later (out-of-order race)", async () => {
    const manifestOf = (t: string) => ({
      schema_version: 1, name: "demo", tag: t, digest: `dig-${t}`, built_at: "t",
      parents: [], plugins: [], requires_secrets: [],
      harness: { type: "claude", model: `model-${t}`, interactive: false }, env: {}, policy: {},
      evals: [], layers: [],
    });

    (getTags as ReturnType<typeof vi.fn>).mockResolvedValue({
      name: "demo",
      tags: [
        { tag: "a", digest: "digA", built_at: "t", pushed_at: "t" },
        { tag: "b", digest: "digB", built_at: "t", pushed_at: "t" },
      ],
    });

    let resolveA!: (v: unknown) => void;
    let resolveB!: (v: unknown) => void;
    const pendingA = new Promise((res) => { resolveA = res; });
    const pendingB = new Promise((res) => { resolveB = res; });

    (getManifest as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(manifestOf("a")) // auto-select on initial tags load
      .mockImplementationOnce(() => pendingA) // explicit click on tag "a"
      .mockImplementationOnce(() => pendingB); // explicit click on tag "b"

    renderAt("/repo/demo");
    await waitFor(() => expect(screen.getByText(/model-a/)).toBeInTheDocument());

    // Click "a" then immediately "b" — the request for "a" is dispatched first
    // but (per the deferred promises below) resolves *after* the request for "b".
    screen.getByText("digA").closest("tr")!.click();
    screen.getByText("digB").closest("tr")!.click();

    // Resolve out of order: "b" (the later request) settles first, "a" (the
    // stale, superseded request) settles after.
    resolveB(manifestOf("b"));
    resolveA(manifestOf("a"));

    await waitFor(() => expect(screen.getByText(/model-b/)).toBeInTheDocument());
    // The stale "a" response must never overwrite the current tag's manifest.
    expect(screen.queryByText(/model-a/)).not.toBeInTheDocument();
  });
});
