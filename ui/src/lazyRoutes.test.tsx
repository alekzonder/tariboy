import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen } from "@/test/queryRender";
import { MemoryRouter } from "react-router-dom";
import type { ReactNode } from "react";
import { fetchAllAgents } from "@/lib/aggregate";
import App from "./App";

// Page modules that resolve only when the test releases them. With eager
// imports App itself would wait for them (up to the fallback timeout) and
// never show a loading state.
const gates = vi.hoisted(() => {
  const gate = () => {
    let release!: () => void;
    const promise = new Promise<void>((resolve) => {
      release = resolve;
      setTimeout(resolve, 10_000);
    });
    return { promise, release };
  };
  return { images: gate(), console: gate() };
});

vi.mock("@/pages/images/LibraryImagesPage", async () => {
  await gates.images.promise;
  return { default: () => <h1>Images page</h1> };
});

vi.mock("@/pages/agents/AgentConsoleTab", async () => {
  await gates.console.promise;
  return { default: () => <h1>Console tab</h1> };
});

vi.mock("@/lib/aggregate", () => ({ fetchAllAgents: vi.fn() }));

vi.mock("@/components/CustomerQuestionNotifications", () => ({
  CustomerQuestionNotifications: ({ children }: { children: ReactNode }) => children,
}));

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.mocked(fetchAllAgents).mockResolvedValue([
    {
      host: { id: "", label: "This daemon (local)" },
      agents: [{ name: "builder", state: "idle" } as never],
    },
  ]);
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ ok: true, result: { agents: [], count: 0 } }),
    } as Response),
  );
});

describe("lazily loaded screens", () => {
  it("keeps the titlebar and the sidebar while a Library page loads", async () => {
    renderAt("/library/images");

    expect(await screen.findByText("Loading…", { selector: "[role=status]" })).toBeInTheDocument();
    expect(screen.getByTestId("app-titlebar")).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Sections" })).toBeInTheDocument();

    await act(async () => gates.images.release());
    expect(await screen.findByRole("heading", { name: "Images page" })).toBeInTheDocument();
  });

  it("keeps the agent list while an agent tab loads", async () => {
    renderAt("/agents/local/builder/console");

    expect(await screen.findByText("Loading…", { selector: "[role=status]" })).toBeInTheDocument();
    expect(screen.getByRole("complementary", { name: "Navigation" })).toBeInTheDocument();

    await act(async () => gates.console.release());
    expect(await screen.findByRole("heading", { name: "Console tab" })).toBeInTheDocument();
  });
});
