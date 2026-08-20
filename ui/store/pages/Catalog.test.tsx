import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import Catalog from "./Catalog";

vi.mock("../lib/storeApi", () => ({ listRepos: vi.fn() }));
import { listRepos } from "../lib/storeApi";

afterEach(() => vi.clearAllMocks());

describe("Catalog", () => {
  it("renders each repo with its tag count", async () => {
    (listRepos as ReturnType<typeof vi.fn>).mockResolvedValue({
      repos: [
        { name: "demo", tags: ["latest", "v1"] },
        { name: "tools", tags: ["latest"] },
      ],
      count: 2,
    });
    render(
      <MemoryRouter>
        <Catalog />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText("demo")).toBeInTheDocument());
    expect(screen.getByText("tools")).toBeInTheDocument();
    expect(screen.getByText(/2 tags/i)).toBeInTheDocument();
  });

  it("shows an empty state when there are no repos", async () => {
    (listRepos as ReturnType<typeof vi.fn>).mockResolvedValue({ repos: [], count: 0 });
    render(
      <MemoryRouter>
        <Catalog />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText(/no repositories/i)).toBeInTheDocument());
  });
});
