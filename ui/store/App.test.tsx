import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import App from "./App";

vi.mock("./lib/storeApi", () => ({
  getInfo: vi.fn(),
  probeAuth: vi.fn(),
  hasToken: vi.fn(),
  clearToken: vi.fn(),
  listRepos: vi.fn().mockResolvedValue({ repos: [], count: 0 }),
}));
vi.mock("./pages/Login", () => ({ default: () => <div>LOGIN GATE</div> }));
vi.mock("./pages/Catalog", () => ({ default: () => <div>CATALOG</div> }));
vi.mock("./pages/RepoDetail", () => ({ default: () => <div>REPO</div> }));
import { getInfo, probeAuth, hasToken } from "./lib/storeApi";

afterEach(() => vi.clearAllMocks());

describe("App gate", () => {
  it("shows the catalog when anon_pull is enabled", async () => {
    (getInfo as ReturnType<typeof vi.fn>).mockResolvedValue({ version: "9.9", anon_pull: true });
    (probeAuth as ReturnType<typeof vi.fn>).mockResolvedValue(true);
    (hasToken as ReturnType<typeof vi.fn>).mockReturnValue(false);
    render(
      <MemoryRouter initialEntries={["/"]}>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText("CATALOG")).toBeInTheDocument());
  });

  it("shows the login gate when a token is required and the probe fails", async () => {
    (getInfo as ReturnType<typeof vi.fn>).mockResolvedValue({ version: "9.9", anon_pull: false });
    (probeAuth as ReturnType<typeof vi.fn>).mockResolvedValue(false);
    (hasToken as ReturnType<typeof vi.fn>).mockReturnValue(false);
    render(
      <MemoryRouter initialEntries={["/"]}>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText("LOGIN GATE")).toBeInTheDocument());
  });
});
